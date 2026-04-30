using Vortex.Database;
using Vortex.Entities;
using Dapper;

namespace Vortex.Repositories;

public interface IGroupRepository
{
    Task<Group?> GetByIdAsync(long groupId);
    Task<Group?> GetByNameAsync(string name);
    Task<IEnumerable<Group>> GetAllAsync();
    Task<IEnumerable<Group>> GetGroupsByOwnerAsync(long ownerId);
    Task<int> InsertAsync(Group group);
    Task<int> UpdateAsync(Group group);
    Task<int> DeleteAsync(long groupId);
    Task<bool> ExistsAsync(long groupId);
    Task<bool> NameExistsAsync(string name);
}

public class GroupRepository : BaseRepository<Group, long>, IGroupRepository
{
    protected override string TableName => "groups";
    protected override string PrimaryKeyName => "id";
    
    public GroupRepository(IDapperConnectionFactory connectionFactory) 
        : base(connectionFactory)
    {
    }
    
    public async Task<Group?> GetByNameAsync(string name)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT * FROM {TableName} WHERE name = @Name";
            return await connection.QueryFirstOrDefaultAsync<Group>(sql, new { Name = name });
        });
    }
    
    public async Task<IEnumerable<Group>> GetGroupsByOwnerAsync(long ownerId)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT * FROM {TableName} WHERE owner_id = @OwnerId ORDER BY created_at DESC";
            return await connection.QueryAsync<Group>(sql, new { OwnerId = ownerId });
        });
    }
    
    public async Task<bool> ExistsAsync(long groupId)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT COUNT(1) FROM {TableName} WHERE id = @GroupId";
            var count = await connection.ExecuteScalarAsync<int>(sql, new { GroupId = groupId });
            return count > 0;
        });
    }
    
    public async Task<bool> NameExistsAsync(string name)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT COUNT(1) FROM {TableName} WHERE name = @Name";
            var count = await connection.ExecuteScalarAsync<int>(sql, new { Name = name });
            return count > 0;
        });
    }
}
