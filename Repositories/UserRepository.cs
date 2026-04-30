using Vortex.Database;
using Vortex.Entities;
using Dapper;

namespace Vortex.Repositories;

public interface IUserRepository
{
    Task<User?> GetByIdAsync(long userId);
    Task<User?> GetByUsernameAsync(string username);
    Task<User?> GetByPublicIdAsync(string publicId);
    Task<User?> GetByEmailAsync(string email);
    Task<IEnumerable<User>> GetAllAsync();
    Task<int> InsertAsync(User user);
    Task<int> UpdateAsync(User user);
    Task<int> DeleteAsync(long userId);
    Task<bool> ExistsAsync(long userId);
    Task<bool> UsernameExistsAsync(string username);
    Task<bool> EmailExistsAsync(string email);
}

public class UserRepository : BaseRepository<User, long>, IUserRepository
{
    protected override string TableName => "users";
    protected override string PrimaryKeyName => "id";
    
    public UserRepository(IDapperConnectionFactory connectionFactory) 
        : base(connectionFactory)
    {
    }
    
    public async Task<User?> GetByUsernameAsync(string username)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT * FROM {TableName} WHERE username = @Username";
            return await connection.QueryFirstOrDefaultAsync<User>(sql, new { Username = username });
        });
    }
    
    public async Task<User?> GetByPublicIdAsync(string publicId)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT * FROM {TableName} WHERE public_id = @PublicId";
            return await connection.QueryFirstOrDefaultAsync<User>(sql, new { PublicId = publicId });
        });
    }
    
    public async Task<User?> GetByEmailAsync(string email)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT * FROM {TableName} WHERE email = @Email";
            return await connection.QueryFirstOrDefaultAsync<User>(sql, new { Email = email });
        });
    }
    
    public async Task<bool> ExistsAsync(long userId)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT COUNT(1) FROM {TableName} WHERE id = @UserId";
            var count = await connection.ExecuteScalarAsync<int>(sql, new { UserId = userId });
            return count > 0;
        });
    }
    
    public async Task<bool> UsernameExistsAsync(string username)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT COUNT(1) FROM {TableName} WHERE username = @Username";
            var count = await connection.ExecuteScalarAsync<int>(sql, new { Username = username });
            return count > 0;
        });
    }
    
    public async Task<bool> EmailExistsAsync(string email)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT COUNT(1) FROM {TableName} WHERE email = @Email";
            var count = await connection.ExecuteScalarAsync<int>(sql, new { Email = email });
            return count > 0;
        });
    }
    
    public override async Task<int> InsertAsync(User user)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"""
                INSERT INTO {TableName} (username, pwd_hash, email, public_id, created_at, updated_at)
                VALUES (@Username, @PwdHash, @Email, @PublicId, @CreatedAt, @UpdatedAt)
                """;
                
            return await connection.ExecuteAsync(sql, user);
        });
    }
}
