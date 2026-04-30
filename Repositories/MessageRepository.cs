using System.Data;
using Vortex.Database;
using Vortex.Entities;
using Dapper;

namespace Vortex.Repositories;

public interface IMessageRepository
{
    Task<Message?> GetMessageAsync(string tableName, long msgId);
    Task<IEnumerable<Message>> GetConversationMessagesAsync(string tableName, long convId, int limit, int offset);
    Task<IEnumerable<Message>> GetMessagesByUserAsync(string tableName, long userId, int limit, int offset);
    Task<int> InsertMessageAsync(string tableName, Message message);
    Task<int> InsertMessagesAsync(string tableName, IEnumerable<Message> messages);
    Task<int> UpdateMessageAsync(string tableName, Message message);
    Task<int> DeleteMessageAsync(string tableName, long msgId);
    Task<int> GetMessageCountAsync(string tableName, long convId);
    Task<bool> MessageExistsAsync(string tableName, long msgId);
    bool IsValidTableName(string tableName);
    Task<int> CreateMessageTableAsync(string tableName);
    Task<int> DropMessageTableAsync(string tableName);
    Task<long> GetMaxMessageIdAsync(string tableName);
    Task<List<long>> GetMaxMessageIdsFromRecentTablesAsync(int days);
}

public class MessageRepository : BaseRepository<Message, long>, IMessageRepository
{
    protected override string TableName => "messages";
    protected override string PrimaryKeyName => "msg_id";

    public MessageRepository(IDapperConnectionFactory connectionFactory)
        : base(connectionFactory)
    {
    }

    public bool IsValidTableName(string tableName)
    {
        return TableNameValidator.IsValidMessageTableName(tableName);
    }

    public async Task<Message?> GetMessageAsync(string tableName, long msgId)
    {
        if (!IsValidTableName(tableName))
            return null;

        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT * FROM {tableName} WHERE msg_id = @MsgId";
            return await connection.QueryFirstOrDefaultAsync<Message>(sql, new { MsgId = msgId });
        });
    }

    public async Task<IEnumerable<Message>> GetConversationMessagesAsync(string tableName, long convId, int limit, int offset)
    {
        if (!IsValidTableName(tableName))
            return Enumerable.Empty<Message>();

        return await ExecuteAsync(async connection =>
        {
            var sql = $"""
                SELECT * FROM {tableName} 
                WHERE conv_id = @ConvId 
                ORDER BY ts DESC 
                LIMIT @Limit OFFSET @Offset
                """;

            return await connection.QueryAsync<Message>(sql, new
            {
                ConvId = convId,
                Limit = limit,
                Offset = offset
            });
        });
    }

    public async Task<IEnumerable<Message>> GetMessagesByUserAsync(string tableName, long userId, int limit, int offset)
    {
        if (!IsValidTableName(tableName))
            return Enumerable.Empty<Message>();

        return await ExecuteAsync(async connection =>
        {
            var sql = $"""
                SELECT * FROM {tableName} 
                WHERE from_uid = @UserId 
                ORDER BY ts DESC 
                LIMIT @Limit OFFSET @Offset
                """;

            return await connection.QueryAsync<Message>(sql, new
            {
                UserId = userId,
                Limit = limit,
                Offset = offset
            });
        });
    }

    public async Task<int> InsertMessageAsync(string tableName, Message message)
    {
        if (!IsValidTableName(tableName))
            return 0;

        return await ExecuteAsync(async connection =>
        {
            var sql = $"""
                INSERT INTO {tableName} (msg_id, conv_id, from_uid, content, ts, is_recalled)
                VALUES (@MsgId, @ConvId, @FromUid, @Content, @Ts, @IsRecalled)
                """;

            return await connection.ExecuteAsync(sql, message);
        });
    }

    public async Task<int> InsertMessagesAsync(string tableName, IEnumerable<Message> messages)
    {
        if (!IsValidTableName(tableName))
            return 0;

        return await ExecuteAsync(async connection =>
        {
            var sql = $"""
                INSERT INTO {tableName} (msg_id, conv_id, from_uid, content, ts, is_recalled)
                VALUES (@MsgId, @ConvId, @FromUid, @Content, @Ts, @IsRecalled)
                """;

            return await connection.ExecuteAsync(sql, messages);
        });
    }

    public async Task<int> UpdateMessageAsync(string tableName, Message message)
    {
        if (!IsValidTableName(tableName))
            return 0;

        return await ExecuteAsync(async connection =>
        {
            var sql = $"""
                UPDATE {tableName} 
                SET conv_id = @ConvId, from_uid = @FromUid, content = @Content, 
                    ts = @Ts, is_recalled = @IsRecalled
                WHERE msg_id = @MsgId
                """;

            return await connection.ExecuteAsync(sql, message);
        });
    }

    public async Task<int> DeleteMessageAsync(string tableName, long msgId)
    {
        if (!IsValidTableName(tableName))
            return 0;

        return await ExecuteAsync(async connection =>
        {
            var sql = $"DELETE FROM {tableName} WHERE msg_id = @MsgId";
            return await connection.ExecuteAsync(sql, new { MsgId = msgId });
        });
    }

    public async Task<int> GetMessageCountAsync(string tableName, long convId)
    {
        if (!IsValidTableName(tableName))
            return 0;

        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT COUNT(*) FROM {tableName} WHERE conv_id = @ConvId";
            return await connection.ExecuteScalarAsync<int>(sql, new { ConvId = convId });
        });
    }

    public async Task<bool> MessageExistsAsync(string tableName, long msgId)
    {
        if (!IsValidTableName(tableName))
            return false;

        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT COUNT(1) FROM {tableName} WHERE msg_id = @MsgId";
            var count = await connection.ExecuteScalarAsync<int>(sql, new { MsgId = msgId });
            return count > 0;
        });
    }

    public async Task<int> CreateMessageTableAsync(string tableName)
    {
        if (!IsValidTableName(tableName))
            return 0;

        return await ExecuteAsync(async connection =>
        {
            var sql = $"""
                CREATE TABLE IF NOT EXISTS {tableName} (
                    msg_id BIGINT PRIMARY KEY,
                    conv_id BIGINT NOT NULL,
                    from_uid BIGINT NOT NULL,
                    content TEXT NOT NULL,
                    ts BIGINT NOT NULL,
                    is_recalled BOOLEAN NOT NULL DEFAULT FALSE
                )
                """;

            return await connection.ExecuteAsync(sql);
        });
    }

    public async Task<int> DropMessageTableAsync(string tableName)
    {
        if (!IsValidTableName(tableName))
            return 0;

        return await ExecuteAsync(async connection =>
        {
            var sql = $"DROP TABLE IF EXISTS {tableName}";
            return await connection.ExecuteAsync(sql);
        });
    }

    public async Task<long> GetMaxMessageIdAsync(string tableName)
    {
        if (!IsValidTableName(tableName))
            return 0;

        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT MAX(msg_id) FROM {tableName}";
            var result = await connection.ExecuteScalarAsync<long?>(sql);
            return result ?? 0;
        });
    }

    public async Task<List<long>> GetMaxMessageIdsFromRecentTablesAsync(int days)
    {
        var today = DateOnly.FromDateTime(DateTime.UtcNow);
        var tableNames = Enumerable.Range(0, days)
            .Select(i => today.AddDays(-i))
            .Select(date => $"messages_{date:yyyyMMdd}")
            .Where(IsValidTableName)
            .ToList();

        var maxIds = new List<long>();

        foreach (var tableName in tableNames)
        {
            try
            {
                var maxId = await GetMaxMessageIdAsync(tableName);
                if (maxId > 0)
                {
                    maxIds.Add(maxId);
                }
            }
            catch
            {
            }
        }

        return maxIds;
    }
}
