using System.Data.Common;

namespace Vortex.Database;

public interface IDatabaseMaintenanceStrategy
{
    Task<bool> WalCheckpointAsync(DbConnection connection);
    Task<bool> IncrementalVacuumAsync(DbConnection connection, int? pages = null);
    Task<bool> FullVacuumAsync(DbConnection connection);
    Task<bool> PurgeOldDataAsync(DbConnection connection, string tableName);
}
