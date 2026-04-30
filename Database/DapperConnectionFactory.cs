using System.Data.Common;
using Microsoft.Data.Sqlite;

namespace Vortex.Database;

public interface IDapperConnectionFactory
{
    DbConnection CreateConnection();
    string GetConnectionString();
}

public class SqliteDapperConnectionFactory : IDapperConnectionFactory
{
    private readonly string _connectionString;
    
    public SqliteDapperConnectionFactory(string connectionString)
    {
        _connectionString = connectionString ?? throw new ArgumentNullException(nameof(connectionString));
    }
    
    public DbConnection CreateConnection()
    {
        var connection = new SqliteConnection(_connectionString);
        connection.Open();
        return connection;
    }
    
    public string GetConnectionString() => _connectionString;
}

public class PostgresDapperConnectionFactory : IDapperConnectionFactory
{
    private readonly string _connectionString;
    
    public PostgresDapperConnectionFactory(string connectionString)
    {
        _connectionString = connectionString ?? throw new ArgumentNullException(nameof(connectionString));
    }
    
    public DbConnection CreateConnection()
    {
        throw new NotSupportedException("PostgreSQL is not supported. Please install Npgsql package.");
    }
    
    public string GetConnectionString() => _connectionString;
}
