using Vortex.Database;
using Vortex.Entities;
using Dapper;

namespace Vortex.Repositories;

public interface IFriendRequestRepository
{
    Task<FriendRequest?> GetByIdAsync(long requestId);
    Task<FriendRequest?> GetByUsersAsync(long fromUserId, long toUserId);
    Task<IEnumerable<FriendRequest>> GetSentRequestsAsync(long fromUserId);
    Task<IEnumerable<FriendRequest>> GetReceivedRequestsAsync(long toUserId);
    Task<IEnumerable<FriendRequest>> GetPendingRequestsAsync(long userId);
    Task<int> InsertAsync(FriendRequest request);
    Task<int> UpdateAsync(FriendRequest request);
    Task<int> DeleteAsync(long requestId);
    Task<int> DeleteByUsersAsync(long fromUserId, long toUserId);
    Task<bool> ExistsAsync(long requestId);
    Task<bool> RequestExistsAsync(long fromUserId, long toUserId);
}

public class FriendRequestRepository : BaseRepository<FriendRequest, long>, IFriendRequestRepository
{
    protected override string TableName => "friend_requests";
    protected override string PrimaryKeyName => "id";
    
    public FriendRequestRepository(IDapperConnectionFactory connectionFactory) 
        : base(connectionFactory)
    {
    }
    
    public async Task<FriendRequest?> GetByUsersAsync(long fromUserId, long toUserId)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT * FROM {TableName} WHERE from_user_id = @FromUserId AND to_user_id = @ToUserId";
            return await connection.QueryFirstOrDefaultAsync<FriendRequest>(sql, new 
            { 
                FromUserId = fromUserId, 
                ToUserId = toUserId 
            });
        });
    }
    
    public async Task<IEnumerable<FriendRequest>> GetSentRequestsAsync(long fromUserId)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT * FROM {TableName} WHERE from_user_id = @FromUserId ORDER BY created_at DESC";
            return await connection.QueryAsync<FriendRequest>(sql, new { FromUserId = fromUserId });
        });
    }
    
    public async Task<IEnumerable<FriendRequest>> GetReceivedRequestsAsync(long toUserId)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT * FROM {TableName} WHERE to_user_id = @ToUserId ORDER BY created_at DESC";
            return await connection.QueryAsync<FriendRequest>(sql, new { ToUserId = toUserId });
        });
    }
    
    public async Task<IEnumerable<FriendRequest>> GetPendingRequestsAsync(long userId)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT * FROM {TableName} WHERE to_user_id = @UserId AND status = 'pending' ORDER BY created_at DESC";
            return await connection.QueryAsync<FriendRequest>(sql, new { UserId = userId });
        });
    }
    
    public async Task<int> DeleteByUsersAsync(long fromUserId, long toUserId)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"DELETE FROM {TableName} WHERE from_user_id = @FromUserId AND to_user_id = @ToUserId";
            return await connection.ExecuteAsync(sql, new 
            { 
                FromUserId = fromUserId, 
                ToUserId = toUserId 
            });
        });
    }
    
    public async Task<bool> ExistsAsync(long requestId)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT COUNT(1) FROM {TableName} WHERE id = @RequestId";
            var count = await connection.ExecuteScalarAsync<int>(sql, new { RequestId = requestId });
            return count > 0;
        });
    }
    
    public async Task<bool> RequestExistsAsync(long fromUserId, long toUserId)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT COUNT(1) FROM {TableName} WHERE from_user_id = @FromUserId AND to_user_id = @ToUserId";
            var count = await connection.ExecuteScalarAsync<int>(sql, new 
            { 
                FromUserId = fromUserId, 
                ToUserId = toUserId 
            });
            return count > 0;
        });
    }
}
