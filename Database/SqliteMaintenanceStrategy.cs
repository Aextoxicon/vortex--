using System.Data.Common;
using Microsoft.Data.Sqlite;

namespace Vortex.Database;

public class SqliteMaintenanceStrategy(ILogger<SqliteMaintenanceStrategy> logger) : IDatabaseMaintenanceStrategy
{
    public async Task<bool> WalCheckpointAsync(DbConnection connection)
    {
        try
        {
            await using var command = connection.CreateCommand();
            command.CommandText = "PRAGMA wal_checkpoint(TRUNCATE);";

            var result = await command.ExecuteScalarAsync();

            if (result is not null)
            {
                logger.LogInformation("SQLite WAL checkpoint executed: {Result}", result);
            }

            return true;
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to execute SQLite WAL checkpoint");
            return false;
        }
    }

    public async Task<bool> IncrementalVacuumAsync(DbConnection connection, int? pages = null)
    {
        try
        {
            await using var command = connection.CreateCommand();
            command.CommandText = pages.HasValue
                ? $"PRAGMA incremental_vacuum({pages.Value});"
                : "PRAGMA incremental_vacuum;";

            await command.ExecuteNonQueryAsync();

            logger.LogInformation("SQLite incremental vacuum executed successfully");
            return true;
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to execute SQLite incremental vacuum");
            return false;
        }
    }

    public async Task<bool> FullVacuumAsync(DbConnection connection)
    {
        try
        {
            await using var command = connection.CreateCommand();
            command.CommandText = "VACUUM;";
            await command.ExecuteNonQueryAsync();

            logger.LogInformation("SQLite full vacuum executed successfully");
            return true;
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to execute SQLite full vacuum");
            return false;
        }
    }

    public async Task<bool> PurgeOldDataAsync(DbConnection connection, string tableName)
    {
        try
        {
            await using var command = connection.CreateCommand();
            command.CommandText = $"DROP TABLE IF EXISTS {tableName};";
            await command.ExecuteNonQueryAsync();

            logger.LogInformation("Successfully dropped old SQLite message table: {TableName}", tableName);
            return true;
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to purge old SQLite data");
            return false;
        }
    }

    /// <summary>
    /// Analyze 衻�更新统�俁�
    /// </summary>
    public async Task<bool> AnalyzeAsync(DbConnection connection)
    {
        try
        {
            await using var command = connection.CreateCommand();
            command.CommandText = "ANALYZE;";
            await command.ExecuteNonQueryAsync();

            logger.LogInformation("SQLite analyze executed successfully");
            return true;
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to execute SQLite analyze");
            return false;
        }
    }

    /// <summary>
    /// 获取数据库统访��?    /// </summary>
    public async Task<Dictionary<string, object>> GetStatsAsync(DbConnection connection)
    {
        var stats = new Dictionary<string, object>();

        try
        {
            await using var command = connection.CreateCommand();
            command.CommandText = "PRAGMA page_count;";
            var pageCount = await command.ExecuteScalarAsync();
            stats["page_count"] = pageCount ?? 0;

            command.CommandText = "PRAGMA page_size;";
            var pageSize = await command.ExecuteScalarAsync();
            stats["page_size"] = pageSize ?? 0;

            command.CommandText = "PRAGMA freelist_count;";
            var freelistCount = await command.ExecuteScalarAsync();
            stats["freelist_count"] = freelistCount ?? 0;

            command.CommandText = "PRAGMA wal_checkpoint(PASSIVE);";
            var walResult = await command.ExecuteScalarAsync();
            stats["wal_checkpoint_result"] = walResult ?? "N/A";
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to get SQLite stats");
        }

        return stats;
    }
}
