using Vortex.Database;
using Vortex.Entities;
using Dapper;

namespace Vortex.Repositories;

public interface IIdGeneratorStateRepository
{
    Task<IdGeneratorState?> GetFirstOrDefaultAsync();
    Task<int> InsertAsync(IdGeneratorState state);
    Task<int> UpdateAsync(IdGeneratorState state);
}

public class IdGeneratorStateRepository : BaseRepository<IdGeneratorState, long>, IIdGeneratorStateRepository
{
    protected override string TableName => "id_generator_state";
    protected override string PrimaryKeyName => "id";

    public IdGeneratorStateRepository(IDapperConnectionFactory connectionFactory)
        : base(connectionFactory)
    {
    }

    public async Task<IdGeneratorState?> GetFirstOrDefaultAsync()
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = "SELECT id, last_ts as LastTs, last_seq as LastSeq FROM id_generator_state LIMIT 1";
            return await connection.QueryFirstOrDefaultAsync<IdGeneratorState>(sql);
        });
    }

    public override async Task<int> InsertAsync(IdGeneratorState state)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = @"
                INSERT INTO id_generator_state (last_ts, last_seq) 
                VALUES (@LastTs, @LastSeq);
                SELECT last_insert_rowid();";

            var result = await connection.ExecuteScalarAsync<long>(sql, new { state.LastTs, state.LastSeq });
            return (int)result;
        });
    }

    public override async Task<int> UpdateAsync(IdGeneratorState state)
    {
        return await ExecuteAsync(async connection =>
        {
            var sql = @"
                UPDATE id_generator_state 
                SET last_ts = @LastTs, last_seq = @LastSeq
                WHERE id = @Id";

            return await connection.ExecuteAsync(sql, new { state.Id, state.LastTs, state.LastSeq });
        });
    }
}
