using Dapper;

namespace Vortex.Database;

public class SqliteOptimizer
{
    private static ILogger<SqliteOptimizer>? _logger;

    public SqliteOptimizer(ILogger<SqliteOptimizer> logger)
    {
        _logger = logger;
    }

    public static async Task OptimizeAsync(IDapperConnectionFactory connectionFactory)
    {
        try
        {
            await using var connection = connectionFactory.CreateConnection();
            await connection.OpenAsync();

            await connection.ExecuteAsync("PRAGMA journal_mode=WAL");
            await connection.ExecuteAsync("PRAGMA synchronous=NORMAL");
            await connection.ExecuteAsync("PRAGMA cache_size=-81920");
            await connection.ExecuteAsync("PRAGMA temp_store=MEMORY");
            await connection.ExecuteAsync("PRAGMA automatic_index=OFF");
            await connection.ExecuteAsync("PRAGMA busy_timeout=5000");
            await connection.ExecuteAsync("PRAGMA mmap_size=268435456");
            await connection.ExecuteAsync("PRAGMA locking_mode=NORMAL");
            await connection.ExecuteAsync("PRAGMA auto_vacuum=NONE");
            await connection.ExecuteAsync("PRAGMA optimize=0x01");

            _logger?.LogInformation("SQLite 优化已完成");
        }
        catch (Exception ex)
        {
            _logger?.LogWarning(ex, "SQLite 优化执行失败");
        }
    }

    public static async Task<Dictionary<string, string>> GetSettingsAsync(IDapperConnectionFactory connectionFactory)
    {
        var settings = new Dictionary<string, string>();

        try
        {
            await using var connection = connectionFactory.CreateConnection();
            await connection.OpenAsync();

            var pragmas = new[]
            {
                "journal_mode",
                "synchronous",
                "cache_size",
                "temp_store",
                "automatic_index",
                "busy_timeout",
                "mmap_size",
                "locking_mode",
                "foreign_keys",
                "auto_vacuum"
            };

            foreach (var pragma in pragmas)
            {
                var result = await connection.QueryFirstOrDefaultAsync<string>($"PRAGMA {pragma}");
                settings[pragma] = result ?? "unknown";
            }
        }
        catch (Exception ex)
        {
            _logger?.LogWarning(ex, "获取 SQLite 配置失败");
        }

        return settings;
    }
}
