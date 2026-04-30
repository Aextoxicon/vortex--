using Vortex.Database;
using Vortex.Entities;
using Dapper;

namespace Vortex.Repositories;

public interface IOutboxMessageRepository
{
    Task<OutboxMessage?> GetByIdAsync(long id);
    Task<IEnumerable<OutboxMessage>> GetPendingMessagesAsync(int limit = 100);
    Task<IEnumerable<OutboxMessage>> GetFailedMessagesAsync(int limit = 100);
    Task<int> InsertAsync(OutboxMessage message);
    Task<int> UpdateAsync(OutboxMessage message);
    Task<int> DeleteAsync(long id);
    Task<int> MarkAsProcessedAsync(long id);
    Task<int> MarkAsFailedAsync(long id, string errorMessage);
    Task<int> UpdateRetryInfoAsync(long id, int retryCount, DateTime nextRetryAt);
}

public class OutboxMessageRepository : BaseRepository<OutboxMessage, long>, IOutboxMessageRepository
{
    protected override string TableName => "outbox_messages";
    protected override string PrimaryKeyName => "id";

    public OutboxMessageRepository(IDapperConnectionFactory connectionFactory)
        : base(connectionFactory)
    {
    }

    public async Task<IEnumerable<OutboxMessage>> GetPendingMessagesAsync(int limit = 100)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = @"
                SELECT * FROM outbox_messages 
                WHERE status = 'pending' 
                AND (next_retry_at IS NULL OR next_retry_at <= @Now)
                ORDER BY created_at ASC 
                LIMIT @Limit";

            return await connection.QueryAsync<OutboxMessage>(sql, new
            {
                Now = DateTime.UtcNow,
                Limit = limit
            });
        });
    }

    public async Task<IEnumerable<OutboxMessage>> GetFailedMessagesAsync(int limit = 100)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = @"
                SELECT * FROM outbox_messages 
                WHERE status = 'failed' 
                ORDER BY created_at ASC 
                LIMIT @Limit";

            return await connection.QueryAsync<OutboxMessage>(sql, new { Limit = limit });
        });
    }

    public override async Task<int> InsertAsync(OutboxMessage message)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = @"
                INSERT INTO outbox_messages 
                (msg_id, conv_id, from_uid, content, ts, is_recalled, status, created_at, updated_at) 
                VALUES (@MsgId, @ConvId, @FromUid, @Content, @Ts, @IsRecalled, @Status, @CreatedAt, @UpdatedAt);
                SELECT last_insert_rowid();";

            var result = await connection.ExecuteScalarAsync<long>(sql, message);
            return (int)result;
        });
    }

    public override async Task<int> UpdateAsync(OutboxMessage message)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = @"
                UPDATE outbox_messages 
                SET msg_id = @MsgId, conv_id = @ConvId, from_uid = @FromUid, 
                    content = @Content, ts = @Ts, is_recalled = @IsRecalled, 
                    status = @Status, updated_at = @UpdatedAt
                WHERE id = @Id";

            return await connection.ExecuteAsync(sql, message);
        });
    }

    public async Task<int> MarkAsProcessedAsync(long id)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = @"
                UPDATE outbox_messages 
                SET status = 'processed', updated_at = @UpdatedAt
                WHERE id = @Id";

            return await connection.ExecuteAsync(sql, new
            {
                Id = id,
                UpdatedAt = DateTime.UtcNow
            });
        });
    }

    public async Task<int> MarkAsFailedAsync(long id, string errorMessage)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = @"
                UPDATE outbox_messages 
                SET status = 'failed', error_message = @ErrorMessage, updated_at = @UpdatedAt
                WHERE id = @Id";

            return await connection.ExecuteAsync(sql, new
            {
                Id = id,
                ErrorMessage = errorMessage,
                UpdatedAt = DateTime.UtcNow
            });
        });
    }

    public async Task<int> UpdateRetryInfoAsync(long id, int retryCount, DateTime nextRetryAt)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = @"
                UPDATE outbox_messages 
                SET retry_count = @RetryCount, next_retry_at = @NextRetryAt, updated_at = @UpdatedAt
                WHERE id = @Id";

            return await connection.ExecuteAsync(sql, new
            {
                Id = id,
                RetryCount = retryCount,
                NextRetryAt = nextRetryAt,
                UpdatedAt = DateTime.UtcNow
            });
        });
    }
}
