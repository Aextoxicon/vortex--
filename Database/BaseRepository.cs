using System.Data;
using System.Diagnostics.CodeAnalysis;
using Dapper;

namespace Vortex.Database;

[DynamicallyAccessedMembers(DynamicallyAccessedMemberTypes.PublicProperties)]
public abstract class BaseRepository<[DynamicallyAccessedMembers(DynamicallyAccessedMemberTypes.PublicProperties)] TEntity, TKey> where TEntity : class
{
    protected readonly IDapperConnectionFactory _connectionFactory;
    protected abstract string TableName { get; }
    protected abstract string PrimaryKeyName { get; }
    
    protected BaseRepository(IDapperConnectionFactory connectionFactory)
    {
        _connectionFactory = connectionFactory ?? throw new ArgumentNullException(nameof(connectionFactory));
    }
    
    /// <summary>
    /// 执行数据库操作（自动管理连接）
    /// </summary>
    protected async Task<T> ExecuteAsync<T>(Func<IDbConnection, Task<T>> operation)
    {
        using var connection = _connectionFactory.CreateConnection();
        return await operation(connection);
    }
    
    /// <summary>
    /// 执行数据库操作（无返回值）
    /// </summary>
    protected async Task ExecuteAsync(Func<IDbConnection, Task> operation)
    {
        using var connection = _connectionFactory.CreateConnection();
        await operation(connection);
    }
    
    /// <summary>
    /// 根据主键获取实体
    /// </summary>
    public virtual async Task<TEntity?> GetByIdAsync(TKey id)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT * FROM {TableName} WHERE {PrimaryKeyName} = @Id";
            return await connection.QueryFirstOrDefaultAsync<TEntity>(sql, new { Id = id });
        });
    }
    
    /// <summary>
    /// 获取所有实体
    /// </summary>
    public virtual async Task<IEnumerable<TEntity>> GetAllAsync()
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"SELECT * FROM {TableName}";
            return await connection.QueryAsync<TEntity>(sql);
        });
    }
    
    /// <summary>
    /// 插入实体
    /// </summary>
    public virtual async Task<int> InsertAsync(TEntity entity)
    {
        return await ExecuteAsync(async connection =>
        {
            var properties = typeof(TEntity).GetProperties()
                .Where(p => p.Name != PrimaryKeyName || !IsAutoIncrement())
                .Select(p => p.Name);
                
            var columns = string.Join(", ", properties);
            var parameters = string.Join(", ", properties.Select(p => $"@{p}"));
            
            var sql = $"INSERT INTO {TableName} ({columns}) VALUES ({parameters})";
            return await connection.ExecuteAsync(sql, entity);
        });
    }
    
    /// <summary>
    /// 更新实体
    /// </summary>
    public virtual async Task<int> UpdateAsync(TEntity entity)
    {
        return await ExecuteAsync(async connection =>
        {
            var properties = typeof(TEntity).GetProperties()
                .Where(p => p.Name != PrimaryKeyName)
                .Select(p => $"{p.Name} = @{p.Name}");
                
            var setClause = string.Join(", ", properties);
            var sql = $"UPDATE {TableName} SET {setClause} WHERE {PrimaryKeyName} = @{PrimaryKeyName}";
            
            return await connection.ExecuteAsync(sql, entity);
        });
    }
    
    /// <summary>
    /// 根据主键删除实体
    /// </summary>
    public virtual async Task<int> DeleteAsync(TKey id)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = $"DELETE FROM {TableName} WHERE {PrimaryKeyName} = @Id";
            return await connection.ExecuteAsync(sql, new { Id = id });
        });
    }
    
    /// <summary>
    /// 检查主键是否为自增
    /// </summary>
    protected virtual bool IsAutoIncrement()
    {
        return true; // 默认假设主键为自增
    }
}

/// <summary>
/// 分页查询结果
/// </summary>
public class PaginatedResult<T>
{
    public IEnumerable<T> Items { get; set; } = Enumerable.Empty<T>();
    public int TotalCount { get; set; }
    public int PageNumber { get; set; }
    public int PageSize { get; set; }
    public int TotalPages => (int)Math.Ceiling(TotalCount / (double)PageSize);
    
    public bool HasPreviousPage => PageNumber > 1;
    public bool HasNextPage => PageNumber < TotalPages;
}

/// <summary>
/// 支持分页查询的Repository基类
/// </summary>
public abstract class PagedRepository<[DynamicallyAccessedMembers(DynamicallyAccessedMemberTypes.PublicProperties)] TEntity, TKey> : BaseRepository<TEntity, TKey> where TEntity : class
{
    protected PagedRepository(IDapperConnectionFactory connectionFactory) : base(connectionFactory)
    {
    }
    
    /// <summary>
    /// 分页查询
    /// </summary>
    public virtual async Task<PaginatedResult<TEntity>> GetPagedAsync(int pageNumber, int pageSize, string? orderBy = null)
    {
        return await ExecuteAsync(async connection =>
        {
            var offset = (pageNumber - 1) * pageSize;
            var orderClause = string.IsNullOrEmpty(orderBy) ? $"ORDER BY {PrimaryKeyName} DESC" : $"ORDER BY {orderBy}";
            
            var sql = $"""
                SELECT * FROM {TableName} 
                {orderClause}
                LIMIT @PageSize OFFSET @Offset
                """;
                
            var countSql = $"SELECT COUNT(*) FROM {TableName}";
            
            var items = await connection.QueryAsync<TEntity>(sql, new { PageSize = pageSize, Offset = offset });
            var totalCount = await connection.ExecuteScalarAsync<int>(countSql);
            
            return new PaginatedResult<TEntity>
            {
                Items = items,
                TotalCount = totalCount,
                PageNumber = pageNumber,
                PageSize = pageSize
            };
        });
    }
    
    /// <summary>
    /// 带条件的分页查询
    /// </summary>
    public virtual async Task<PaginatedResult<TEntity>> GetPagedAsync(
        int pageNumber, 
        int pageSize, 
        string whereClause, 
        object parameters, 
        string? orderBy = null)
    {
        return await ExecuteAsync(async connection =>
        {
            var offset = (pageNumber - 1) * pageSize;
            var orderClause = string.IsNullOrEmpty(orderBy) ? $"ORDER BY {PrimaryKeyName} DESC" : $"ORDER BY {orderBy}";
            
            var sql = $"""
                SELECT * FROM {TableName} 
                WHERE {whereClause}
                {orderClause}
                LIMIT @PageSize OFFSET @Offset
                """;
                
            var countSql = $"SELECT COUNT(*) FROM {TableName} WHERE {whereClause}";
            
            // 合并参数
            var mergedParameters = new DynamicParameters(parameters);
            mergedParameters.Add("PageSize", pageSize);
            mergedParameters.Add("Offset", offset);
            
            var items = await connection.QueryAsync<TEntity>(sql, mergedParameters);
            var totalCount = await connection.ExecuteScalarAsync<int>(countSql, parameters);
            
            return new PaginatedResult<TEntity>
            {
                Items = items,
                TotalCount = totalCount,
                PageNumber = pageNumber,
                PageSize = pageSize
            };
        });
    }
}