using System.Collections.Concurrent;

namespace Vortex.Database;

public enum MaintenanceTaskType
{
    WalCheckpoint,
    IncrementalVacuum,
    FullVacuum,
    DataPurge
}

public record MaintenanceTaskConfig(
    MaintenanceTaskType Type,
    string Name,
    Func<Task> Action,
    TimeSpan InitialDelay,
    TimeSpan Period);

public class DatabaseMaintenanceScheduler(
    DatabaseMaintenance maintenance,
    ILogger<DatabaseMaintenanceScheduler> logger) : BackgroundService
{
    private readonly ConcurrentDictionary<MaintenanceTaskType, CancellationTokenSource> _taskTokens = new();

    private List<MaintenanceTaskConfig> GetTaskConfigs() =>
    [
        new MaintenanceTaskConfig(
            MaintenanceTaskType.WalCheckpoint,
            "WAL Checkpoint",
            async () => await maintenance.WalCheckpointAsync(),
            TimeSpan.FromMinutes(5),
            TimeSpan.FromMinutes(30)),

        new MaintenanceTaskConfig(
            MaintenanceTaskType.IncrementalVacuum,
            "Incremental Vacuum",
            async () => await maintenance.IncrementalVacuumAsync(100),
            CalculateDelayUntilNextSunday3AmUtc(),
            TimeSpan.FromDays(7)),

        new MaintenanceTaskConfig(
            MaintenanceTaskType.FullVacuum,
            "Full Vacuum",
            async () => await maintenance.FullVacuumAsync(),
            CalculateDelayUntilNextFirstSaturday3AmUtc(),
            TimeSpan.FromDays(30)),

        new MaintenanceTaskConfig(
            MaintenanceTaskType.DataPurge,
            "Data Purge",
            async () => await maintenance.PurgeOldDataAsync(),
            TimeSpan.FromSeconds(10),
            TimeSpan.FromHours(24))
    ];

    protected override Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var configs = GetTaskConfigs();

        foreach (var config in configs)
        {
            var cts = CancellationTokenSource.CreateLinkedTokenSource(stoppingToken);
            _taskTokens[config.Type] = cts;

            _ = Task.Run(async () => await RunScheduledTaskAsync(config, cts.Token), cts.Token);

            logger.LogInformation("{TaskName} scheduled: initial delay={InitialDelay}, period={Period}",
                config.Name, config.InitialDelay, config.Period);
        }

        return Task.CompletedTask;
    }

    private async Task RunScheduledTaskAsync(MaintenanceTaskConfig config, CancellationToken stoppingToken)
    {
        if (config.InitialDelay > TimeSpan.Zero)
        {
            await Task.Delay(config.InitialDelay, stoppingToken);
        }

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                logger.LogInformation("Executing {TaskName}...", config.Name);
                await config.Action();
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Failed to execute {TaskName}", config.Name);
            }

            await Task.Delay(config.Period, stoppingToken);
        }
    }

    private static TimeSpan CalculateDelayUntilNextSunday3AmUtc()
    {
        var now = DateTime.UtcNow;
        var daysUntilSunday = (7 - (int)now.DayOfWeek) % 7;

        if (daysUntilSunday == 0 && now.Hour >= 3)
        {
            daysUntilSunday = 7;
        }

        var nextSunday = now.Date.AddDays(daysUntilSunday);
        var targetTime = new DateTime(nextSunday.Year, nextSunday.Month, nextSunday.Day, 3, 0, 0, DateTimeKind.Utc);

        return targetTime - now;
    }

    private static TimeSpan CalculateDelayUntilNextFirstSaturday3AmUtc()
    {
        var now = DateTime.UtcNow;
        var nextMonth = now.Month == 12
            ? new DateTime(now.Year + 1, 1, 1)
            : new DateTime(now.Year, now.Month + 1, 1);

        var firstSaturday = nextMonth;
        while (firstSaturday.DayOfWeek != DayOfWeek.Saturday)
        {
            firstSaturday = firstSaturday.AddDays(1);
        }

        var targetTime = new DateTime(firstSaturday.Year, firstSaturday.Month, firstSaturday.Day, 3, 0, 0, DateTimeKind.Utc);

        return targetTime - now;
    }

    public override async Task StopAsync(CancellationToken cancellationToken)
    {
        foreach (var cts in _taskTokens.Values)
        {
            cts.Cancel();
        }

        await base.StopAsync(cancellationToken);
    }
}
