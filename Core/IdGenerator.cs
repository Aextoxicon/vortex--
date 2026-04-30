using System.Collections.Concurrent;
using Vortex.Entities;
using Vortex.Repositories;

namespace Vortex.Core;

public record IdGeneratorState(
    long BaseTs,
    long CurrentTs,
    long CurrentSeq,
    long SegmentEndSeq,
    long LastTs);

public record IdSegment(
    long StartId,
    long EndId,
    long BaseTs,
    long EndTs,
    long NodeId)
{
    public long CurrentId = StartId - 1;
    public int Remaining => (int)(EndId - Interlocked.Read(ref CurrentId));
}

public class IdGenerator(
    IIdGeneratorStateRepository idGeneratorStateRepo,
    IMessageRepository messageRepo,
    DbLock dbLock,
    ILogger<IdGenerator> logger) : BackgroundService
{
    private readonly long _nodeId = AppConfig.NodeId;
    private readonly ConcurrentQueue<IdSegment> _segmentQueue = new();
    private readonly SemaphoreSlim _renewalSemaphore = new(1, 1);
    private readonly object _initLock = new();
    
    private IdGeneratorState? _persistentState;
    private bool _isInitialized;
    private int _prefetchThreshold = AppConfig.IdGenerator.SegmentSize / 4;

    public async Task<(long Id, Error? Error)> GenerateIdAsync()
    {
        await EnsureInitializedAsync();

        while (true)
        {
            if (_segmentQueue.TryPeek(out var segment))
            {
                var id = Interlocked.Increment(ref segment.CurrentId);
                
                if (id <= segment.EndId)
                {
                    TryPrefetchIfNeeded(segment);
                    return (id, null);
                }
                
                _segmentQueue.TryDequeue(out _);
            }
            else
            {
                var fetchError = await FetchNewSegmentAsync();
                if (fetchError != null)
                {
                    return (0, fetchError);
                }
            }
        }
    }

    private void TryPrefetchIfNeeded(IdSegment currentSegment)
    {
        var remaining = currentSegment.Remaining;
        
        if (remaining <= _prefetchThreshold && _segmentQueue.Count < 2)
        {
            Task.Run(async () =>
            {
                try
                {
                    await FetchNewSegmentAsync();
                }
                catch (Exception ex)
                {
                    logger.LogWarning(ex, "Prefetch segment failed");
                }
            });
        }
    }

    private async Task<Error?> FetchNewSegmentAsync()
    {
        if (!await _renewalSemaphore.WaitAsync(0))
        {
            await _renewalSemaphore.WaitAsync();
            _renewalSemaphore.Release();
            return null;
        }

        try
        {
            var now = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
            
            var (newBaseTs, newReservedTs) = await ReserveNewSegmentAsync();

            var startTs = newBaseTs;
            var endTs = newReservedTs - 1;
            
            var startId = (startTs << (17 + 5)) | (_nodeId << 17) | 0;
            var endId = (endTs << (17 + 5)) | (_nodeId << 17) | ((1L << 17) - 1);

            var segment = new IdSegment(startId, endId, startTs, endTs, _nodeId);
            _segmentQueue.Enqueue(segment);

            logger.LogInformation("New segment reserved: start={StartId}, end={EndId}, duration={Duration}ms",
                startId, endId, newReservedTs - newBaseTs);

            return null;
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to fetch new segment");
            return Error.InternalError("Failed to generate ID segment");
        }
        finally
        {
            _renewalSemaphore.Release();
        }
    }

    private async Task<(long BaseTs, long ReservedTs)> ReserveNewSegmentAsync()
    {
        var lockKey = "id_generator_renewal";

        return await dbLock.WithLockAsync(lockKey, async () =>
        {
            var latestState = await LoadStateFromDbAsync();
            var now = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();

            long newBaseTs;
            long newReservedTs;

            if (latestState != null)
            {
                newBaseTs = Math.Max(now, latestState.LastTs);
                newReservedTs = newBaseTs + AppConfig.IdGenerator.SegmentDurationMs;
            }
            else
            {
                var maxExistingIds = await messageRepo.GetMaxMessageIdsFromRecentTablesAsync(7);
                var maxExistingId = maxExistingIds.Count > 0 ? maxExistingIds.Max() : 0;

                var existingTimestampPart = maxExistingId > 0
                    ? ViolenceFlake.ExtractTimestampFromMsgId(maxExistingId)
                    : 0;

                newBaseTs = Math.Max(now, existingTimestampPart + 1);
                newReservedTs = newBaseTs + AppConfig.IdGenerator.SegmentDurationMs;
            }

            var newState = new IdGeneratorState(
                newBaseTs,
                newBaseTs,
                0,
                AppConfig.IdGenerator.SegmentSize,
                newReservedTs);

            await PersistStateToDbAsync(newState);
            _persistentState = newState;

            return (newBaseTs, newReservedTs);
        }, AppConfig.Lock.DefaultTimeoutMs);
    }

    private async Task EnsureInitializedAsync()
    {
        if (_isInitialized) return;

        lock (_initLock)
        {
            if (_isInitialized) return;
        }

        var state = await LoadStateFromDbOrInitAsync();

        try
        {
            await PersistStateToDbAsync(state);
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to persist initial state");
        }

        _persistentState = state;

        var fetchError = await FetchNewSegmentAsync();
        if (fetchError != null)
        {
            logger.LogError("Failed to initialize first segment: {Error}", fetchError.Message);
        }

        lock (_initLock)
        {
            _isInitialized = true;
        }

        logger.LogInformation("ID Generator initialized with node_id: {NodeId}", _nodeId);
    }

    private async Task<IdGeneratorState> LoadStateFromDbOrInitAsync()
    {
        var maxExistingIds = await messageRepo.GetMaxMessageIdsFromRecentTablesAsync(7);
        var maxExistingId = maxExistingIds.Count > 0 ? maxExistingIds.Max() : 0;
        var now = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();

        var dbState = await LoadStateFromDbAsync();

        if (dbState != null)
        {
            return dbState;
        }

        var existingTimestampPart = maxExistingId > 0
            ? ViolenceFlake.ExtractTimestampFromMsgId(maxExistingId)
            : 0;

        var newBaseTs = Math.Max(now, existingTimestampPart + 1);
        var newReservedTs = newBaseTs + AppConfig.IdGenerator.SegmentDurationMs;

        return new IdGeneratorState(
            newBaseTs,
            newBaseTs,
            0,
            AppConfig.IdGenerator.SegmentSize,
            newReservedTs);
    }

    private async Task<IdGeneratorState?> LoadStateFromDbAsync()
    {
        try
        {
            var record = await idGeneratorStateRepo.GetFirstOrDefaultAsync();

            if (record == null)
            {
                return null;
            }

            var baseTs = record.LastTs - AppConfig.IdGenerator.SegmentDurationMs;

            return new IdGeneratorState(
                baseTs,
                baseTs,
                record.LastSeq,
                AppConfig.IdGenerator.SegmentSize,
                record.LastTs);
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to load ID generator state from database");
            return null;
        }
    }

    private async Task PersistStateToDbAsync(IdGeneratorState state)
    {
        var existing = await idGeneratorStateRepo.GetFirstOrDefaultAsync();

        if (existing != null)
        {
            existing.LastTs = state.LastTs;
            existing.LastSeq = state.CurrentSeq;
            await idGeneratorStateRepo.UpdateAsync(existing);
        }
        else
        {
            var record = new Entities.IdGeneratorState
            {
                LastTs = state.LastTs,
                LastSeq = state.CurrentSeq
            };
            await idGeneratorStateRepo.InsertAsync(record);
        }
    }

    public long GetNodeId() => _nodeId;

    public long ExtractTimestampFromMsgId(long msgId) =>
        ViolenceFlake.ExtractTimestampFromMsgId(msgId);

    public long ExtractNodeIdFromMsgId(long msgId) =>
        ViolenceFlake.ExtractNodeIdFromMsgId(msgId);

    public long ExtractSequenceFromMsgId(long msgId) =>
        ViolenceFlake.ExtractSequenceFromMsgId(msgId);

    public int GetQueueDepth() => _segmentQueue.Count;

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        await EnsureInitializedAsync();

        using var timer = new PeriodicTimer(TimeSpan.FromSeconds(30));

        while (!stoppingToken.IsCancellationRequested && await timer.WaitForNextTickAsync(stoppingToken))
        {
            var queueDepth = _segmentQueue.Count;
            if (queueDepth < 2)
            {
                logger.LogDebug("Segment queue depth: {Depth}, prefetching...", queueDepth);
                await FetchNewSegmentAsync();
            }
        }
    }
}
