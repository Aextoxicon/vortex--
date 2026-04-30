using Dapper;
using Microsoft.Extensions.DependencyInjection;

namespace Vortex.Core;

public class DbLock(IServiceScopeFactory scopeFactory)
{
    public async Task<T?> WithLockAsync<T>(string lockKey, Func<Task<T>> func, int? timeout = null)
    {
        timeout ??= AppConfig.Lock.DefaultTimeoutMs;
        var adapter = await GetAdapter();

        return adapter switch
        {
            "NpgsqlConnection" => await WithPostgresLockAsync(lockKey, func, timeout.Value),
            "SqliteConnection" => await WithSqliteLockAsync(lockKey, func, timeout.Value),
            _ => throw new NotSupportedException($"Unsupported database adapter: {adapter}")
        };
    }

    public async Task WithLockAsync(string lockKey, Func<Task> func, int? timeout = null)
    {
        await WithLockAsync(lockKey, async () =>
        {
            await func();
            return default(object?);
        }, timeout);
    }

    private async Task<T?> WithPostgresLockAsync<T>(string lockKey, Func<Task<T>> func, int timeout)
    {
        using var scope = scopeFactory.CreateScope();
        var connectionFactory = scope.ServiceProvider.GetRequiredService<Database.IDapperConnectionFactory>();
        var lockId = LockKeyToId(lockKey);
        var startTime = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();

        while (true)
        {
            await using var connection = connectionFactory.CreateConnection();
            await connection.OpenAsync();

            var acquired = await connection.ExecuteScalarAsync<int>(
                "SELECT pg_try_advisory_lock(@LockId)", new { LockId = lockId });

            if (acquired > 0)
            {
                try
                {
                    return await func();
                }
                finally
                {
                    await connection.ExecuteAsync(
                        "SELECT pg_advisory_unlock(@LockId)", new { LockId = lockId });
                }
            }

            var elapsed = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds() - startTime;
            if (elapsed >= timeout)
            {
                throw new TimeoutException($"Lock acquisition timeout: {lockKey}");
            }

            await Task.Delay(AppConfig.Lock.RetryIntervalMs);
        }
    }

    private async Task<T?> WithSqliteLockAsync<T>(string lockKey, Func<Task<T>> func, int timeout)
    {
        using var scope = scopeFactory.CreateScope();
        var connectionFactory = scope.ServiceProvider.GetRequiredService<Database.IDapperConnectionFactory>();
        await EnsureLockTableExistsAsync();

        var lockId = LockKeyToId(lockKey);
        var startTime = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();

        while (true)
        {
            await using var connection = connectionFactory.CreateConnection();
            await connection.OpenAsync();
            await using var transaction = await connection.BeginTransactionAsync();

            try
            {
                var now = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
                await connection.ExecuteAsync(
                    "DELETE FROM distributed_locks WHERE expires_at < @Now", 
                    new { Now = now }, transaction);

                var existingLock = await connection.QueryFirstOrDefaultAsync<LockRecord>(
                    "SELECT lock_id, owner_id, expires_at FROM distributed_locks WHERE lock_id = @LockId",
                    new { LockId = lockId }, transaction);

                var expiresAt = now + timeout;

                if (existingLock == null)
                {
                    await connection.ExecuteAsync(
                        "INSERT INTO distributed_locks (lock_id, owner_id, expires_at) VALUES (@LockId, @OwnerId, @ExpiresAt)",
                        new { LockId = lockId, OwnerId = Environment.ProcessId, ExpiresAt = expiresAt }, transaction);

                    await transaction.CommitAsync();
                    try { return await func(); }
                    finally { await ReleaseSqliteLockAsync(lockId); }
                }
                else if (existingLock.ExpiresAt < now)
                {
                    await connection.ExecuteAsync(
                        "UPDATE distributed_locks SET owner_id = @OwnerId, expires_at = @ExpiresAt WHERE lock_id = @LockId",
                        new { OwnerId = Environment.ProcessId, ExpiresAt = expiresAt, LockId = lockId }, transaction);

                    await transaction.CommitAsync();
                    try { return await func(); }
                    finally { await ReleaseSqliteLockAsync(lockId); }
                }
                else
                {
                    await transaction.RollbackAsync();
                    var elapsed = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds() - startTime;

                    if (elapsed >= timeout)
                    {
                        throw new TimeoutException($"Lock acquisition timeout: {lockKey}");
                    }

                    await Task.Delay(AppConfig.Lock.RetryIntervalMs);
                }
            }
            catch
            {
                await transaction.RollbackAsync();
                throw;
            }
        }
    }

    private async Task ReleaseSqliteLockAsync(long lockId)
    {
        using var scope = scopeFactory.CreateScope();
        var connectionFactory = scope.ServiceProvider.GetRequiredService<Database.IDapperConnectionFactory>();
        await using var connection = connectionFactory.CreateConnection();
        await connection.OpenAsync();
        await connection.ExecuteAsync(
            "DELETE FROM distributed_locks WHERE lock_id = @LockId AND owner_id = @OwnerId",
            new { LockId = lockId, OwnerId = Environment.ProcessId });
    }

    private async Task EnsureLockTableExistsAsync()
    {
        using var scope = scopeFactory.CreateScope();
        var connectionFactory = scope.ServiceProvider.GetRequiredService<Database.IDapperConnectionFactory>();
        await using var connection = connectionFactory.CreateConnection();
        await connection.OpenAsync();
        
        var createTableSql = """
            CREATE TABLE IF NOT EXISTS distributed_locks (
                lock_id INTEGER PRIMARY KEY,
                owner_id INTEGER NOT NULL,
                expires_at INTEGER NOT NULL
            )
            """;

        await connection.ExecuteAsync(createTableSql);
    }

    private async Task<string> GetAdapter()
    {
        using var scope = scopeFactory.CreateScope();
        var connectionFactory = scope.ServiceProvider.GetRequiredService<Database.IDapperConnectionFactory>();
        await using var connection = connectionFactory.CreateConnection();
        return connection.GetType().Name;
    }

    private static long LockKeyToId(string lockKey)
    {
        return lockKey.GetHashCode() & 0x7FFFFFFF;
    }
}

public class LockRecord
{
    public long LockId { get; set; }
    public long OwnerId { get; set; }
    public long ExpiresAt { get; set; }
}
