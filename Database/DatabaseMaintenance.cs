using System.Data.Common;
using Microsoft.Extensions.DependencyInjection;

namespace Vortex.Database;

public class DatabaseMaintenance(
    IServiceScopeFactory scopeFactory,
    IDatabaseMaintenanceStrategy strategy,
    ILogger<DatabaseMaintenance> logger)
{
    private readonly SemaphoreSlim _lock = new(1, 1);

    private async Task<bool> ExecuteLockedAsync(string operationName, Func<DbConnection, Task<bool>> operation)
    {
        if (!await _lock.WaitAsync(0))
        {
            logger.LogWarning("{Operation} skipped, another task is running", operationName);
            return false;
        }

        try
        {
            using var scope = scopeFactory.CreateScope();
            var connectionFactory = scope.ServiceProvider.GetRequiredService<IDapperConnectionFactory>();
            await using var connection = connectionFactory.CreateConnection();
            await connection.OpenAsync();
            return await operation(connection);
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to execute {Operation}", operationName);
            return false;
        }
        finally
        {
            _lock.Release();
        }
    }

    public Task<bool> WalCheckpointAsync() =>
        ExecuteLockedAsync("WAL checkpoint", connection => strategy.WalCheckpointAsync(connection));

    public Task<bool> IncrementalVacuumAsync(int? pages = null) =>
        ExecuteLockedAsync("Incremental vacuum", connection => strategy.IncrementalVacuumAsync(connection, pages));

    public Task<bool> FullVacuumAsync() =>
        ExecuteLockedAsync("Full vacuum", connection => strategy.FullVacuumAsync(connection));

    public async Task<bool> PurgeOldDataAsync()
    {
        var today = DateOnly.FromDateTime(DateTime.UtcNow);
        var cutoffDate = today.AddDays(-7);
        var purgeDate = cutoffDate.AddDays(-1);
        var tableName = $"messages_{purgeDate:yyyyMMdd}";

        if (!TableNameValidator.IsValidMessageTableName(tableName))
        {
            logger.LogError("Invalid table name generated for purge: {TableName}", tableName);
            return false;
        }

        return await ExecuteLockedAsync("Data purge", connection => strategy.PurgeOldDataAsync(connection, tableName));
    }
}
