namespace Vortex.Core;

public static class ViolenceFlake
{
    private const int TimestampBits = 41;
    private const int NodeIdBits = 5;
    private const int SequenceBits = 17;

    private const long MaxTimestamp = (1L << TimestampBits) - 1;
    private const long MaxNodeId = (1L << NodeIdBits) - 1;
    private const long MaxSequence = (1L << SequenceBits) - 1;

    private const int NodeIdShift = SequenceBits;
    private const int TimestampShift = SequenceBits + NodeIdBits;

    public static (long Id, long NewTs, long NewSeq) CalculateNextId(
        long currentTs, long currentSeq, long nodeId)
    {
        if (nodeId < 0 || nodeId > MaxNodeId)
        {
            throw new ArgumentException($"Node ID must be between 0 and {MaxNodeId}");
        }

        long newTs = currentTs;
        long newSeq = currentSeq;

        if (currentSeq < MaxSequence)
        {
            newSeq = currentSeq + 1;
        }
        else
        {
            newTs = currentTs + 1;
            newSeq = 0;
        }

        if (newTs > MaxTimestamp)
        {
            throw new InvalidOperationException("Timestamp overflow");
        }

        var id = ((newTs << TimestampShift) |
                  (nodeId << NodeIdShift) |
                  newSeq);

        return (id, newTs, newSeq);
    }

    public static long ExtractTimestampFromMsgId(long msgId)
    {
        return (msgId >> TimestampShift) & MaxTimestamp;
    }

    public static long ExtractNodeIdFromMsgId(long msgId)
    {
        return (msgId >> NodeIdShift) & MaxNodeId;
    }

    public static long ExtractSequenceFromMsgId(long msgId)
    {
        return msgId & MaxSequence;
    }
}
