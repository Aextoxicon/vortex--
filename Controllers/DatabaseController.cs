using Microsoft.AspNetCore.Mvc;
using Vortex.Database;
using Dapper;

namespace Vortex.Controllers;

[Route("api/[controller]")]
public class DatabaseController : BaseController
{
    private readonly IDapperConnectionFactory _connectionFactory;
    private readonly ILogger<DatabaseController> _logger;

    public DatabaseController(IDapperConnectionFactory connectionFactory, ILogger<DatabaseController> logger)
    {
        _connectionFactory = connectionFactory;
        _logger = logger;
    }

    [HttpGet("stats")]
    public async Task<IActionResult> GetStats()
    {
        try
        {
            await using var connection = _connectionFactory.CreateConnection();
            await connection.OpenAsync();

            var sql = """
                SELECT 
                    (SELECT COUNT(*) FROM users) as user_count,
                    (SELECT COUNT(*) FROM groups) as group_count,
                    (SELECT COUNT(*) FROM friend_requests) as friend_request_count
                """;

            var stats = await connection.QueryFirstOrDefaultAsync<DatabaseStats>(sql);
            return Ok(stats);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to get database stats");
            return StatusCode(500, new ErrorResponse("Failed to get database stats"));
        }
    }

    [HttpPost("analyze")]
    public async Task<IActionResult> Analyze()
    {
        try
        {
            await using var connection = _connectionFactory.CreateConnection();
            await connection.OpenAsync();

            await connection.ExecuteAsync("ANALYZE");
            return Ok(new SuccessResponse(true, "Database analyzed successfully"));
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to analyze database");
            return StatusCode(500, new ErrorResponse("Failed to analyze database"));
        }
    }

    [HttpPost("checkpoint")]
    public async Task<IActionResult> Checkpoint()
    {
        try
        {
            await using var connection = _connectionFactory.CreateConnection();
            await connection.OpenAsync();

            await connection.ExecuteAsync("PRAGMA wal_checkpoint(TRUNCATE)");
            return Ok(new SuccessResponse(true, "WAL checkpoint completed"));
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to perform checkpoint");
            return StatusCode(500, new ErrorResponse("Failed to perform checkpoint"));
        }
    }
}
