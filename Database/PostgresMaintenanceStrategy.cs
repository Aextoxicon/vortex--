using System.Data.Common;

namespace Vortex.Database;

public class PostgresMaintenanceStrategy(ILogger<PostgresMaintenanceStrategy> logger) : IDatabaseMaintenanceStrategy
{
    public async Task<bool> WalCheckpointAsync(DbConnection connection)
    {
        try
        {
            await using var command = connection.CreateCommand();
            command.CommandText = "CHECKPOINT;";
            await command.ExecuteNonQueryAsync();

            logger.LogInformation("PostgreSQL checkpoint executed successfully");
            return true;
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to execute PostgreSQL checkpoint");
            return false;
        }
    }

    public async Task<bool> IncrementalVacuumAsync(DbConnection connection, int? pages = null)
    {
        try
        {
            await using var command = connection.CreateCommand();
            command.CommandText = "VACUUM;";
            await command.ExecuteNonQueryAsync();

            logger.LogInformation("PostgreSQL vacuum executed successfully");
            return true;
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to execute PostgreSQL vacuum");
            return false;
        }
    }

    public async Task<bool> FullVacuumAsync(DbConnection connection)
    {
        try
        {
            await using var command = connection.CreateCommand();
            command.CommandText = "VACUUM FULL;";
            await command.ExecuteNonQueryAsync();

            logger.LogInformation("PostgreSQL full vacuum executed successfully");
            return true;
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to execute PostgreSQL full vacuum");
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

            logger.LogInformation("Successfully dropped old PostgreSQL message table: {TableName}", tableName);
            return true;
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to purge old PostgreSQL data");
            return false;
        }
    }
}
