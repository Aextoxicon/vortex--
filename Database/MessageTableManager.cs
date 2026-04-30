using Dapper;
using Microsoft.Extensions.DependencyInjection;

namespace Vortex.Database;

public class MessageTableManager(IServiceScopeFactory scopeFactory, ILogger<MessageTableManager> logger) : BackgroundService
{
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.LogInformation("MessageTableManager started");

        await CreateTablesFromTodayToSundayAsync();

        while (!stoppingToken.IsCancellationRequested)
        {
            var delay = CalculateDelayUntilNextMondayUtc();

            if (delay > TimeSpan.Zero)
            {
                logger.LogInformation("Next table creation scheduled for {Delay}", delay);
                await Task.Delay(delay, stoppingToken);
            }

            if (!stoppingToken.IsCancellationRequested)
            {
                logger.LogInformation("Creating weekly message tables...");
                await CreateWeekTablesAsync();
            }
        }
    }

    private async Task CreateTablesFromTodayToSundayAsync()
    {
        var today = DateOnly.FromDateTime(DateTime.UtcNow);
        var dayOfWeek = (int)today.DayOfWeek;
        var daysToSunday = dayOfWeek == 0 ? 0 : 7 - dayOfWeek;

        logger.LogInformation("Creating tables from today to Sunday ({Days} days)...", daysToSunday);

        for (var offset = 0; offset <= daysToSunday; offset++)
        {
            var date = today.AddDays(offset);
            var tableName = $"messages_{date:yyyyMMdd}";
            await CreateTableIfNotExistsAsync(tableName);
        }

        logger.LogInformation("Tables created successfully");
    }

    private async Task CreateWeekTablesAsync()
    {
        var today = DateOnly.FromDateTime(DateTime.UtcNow);
        var dayOfWeek = (int)today.DayOfWeek;
        var daysSinceMonday = dayOfWeek == 0 ? 6 : dayOfWeek - 1;
        var monday = today.AddDays(-daysSinceMonday);

        logger.LogInformation("Creating week tables from {Monday}...", monday);

        for (var offset = 0; offset < 7; offset++)
        {
            var date = monday.AddDays(offset);
            var tableName = $"messages_{date:yyyyMMdd}";
            await CreateTableIfNotExistsAsync(tableName);
        }

        logger.LogInformation("Week tables created successfully");
    }

    private async Task CreateTableIfNotExistsAsync(string tableName)
    {
        if (!IsValidTableName(tableName))
        {
            logger.LogError("Invalid table name: {TableName}", tableName);
            return;
        }

        var sql = $"""
            CREATE TABLE IF NOT EXISTS {tableName} (
                msg_id BIGINT PRIMARY KEY,
                conv_id VARCHAR NOT NULL,
                from_uid INTEGER NOT NULL,
                content TEXT NOT NULL,
                ts BIGINT NOT NULL,
                is_recalled INTEGER DEFAULT 0
            );
            CREATE INDEX IF NOT EXISTS idx_msg_conv_sync_{tableName} ON {tableName} (conv_id, msg_id);
            CREATE INDEX IF NOT EXISTS idx_msg_id_from_uid_{tableName} ON {tableName} (msg_id, from_uid);
            CREATE INDEX IF NOT EXISTS idx_msg_conv_ts_{tableName} ON {tableName} (conv_id, ts DESC);
            CREATE INDEX IF NOT EXISTS idx_msg_from_uid_{tableName} ON {tableName} (from_uid);
            """;

        try
        {
            using var scope = scopeFactory.CreateScope();
            var connectionFactory = scope.ServiceProvider.GetRequiredService<IDapperConnectionFactory>();
            await using var connection = connectionFactory.CreateConnection();
            await connection.OpenAsync();
            await connection.ExecuteAsync(sql);
            logger.LogDebug("Table {TableName} ensured", tableName);
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to create table {TableName}", tableName);
        }
    }

    private static bool IsValidTableName(string tableName)
    {
        return TableNameValidator.IsValidMessageTableName(tableName);
    }

    private static TimeSpan CalculateDelayUntilNextMondayUtc()
    {
        var now = DateTime.UtcNow;
        var dayOfWeek = (int)now.DayOfWeek;
        var daysUntilMonday = dayOfWeek == 1 ? 7 : (7 - dayOfWeek) + 1;

        var nextMonday = now.Date.AddDays(daysUntilMonday);
        var targetTime = new DateTime(nextMonday.Year, nextMonday.Month, nextMonday.Day, 0, 0, 0, DateTimeKind.Utc);

        return targetTime - now;
    }
}
