namespace Vortex;

public static class AppConfig
{
    public static int NodeId { get; set; } = 0;

    public static class Time
    {
        public const long EpochTime = 1_767_225_600_000;
        public const int RateLimitWindowMs = 1_000;
        public const int MessageRecallWindowMs = 120_000;
    }

    public static class Cache
    {
        public const int RateLimitTtlSeconds = 2;
        public const int SessionPermTtlMinutes = 10;
        public const int GroupTtlMinutes = 2;
        public const int MaxSize = 10_000;
    }

    public static class Message
    {
        public const int BatchSize = 50;
        public const int BatchTimeoutMs = 100;
        public const int BackpressureThreshold = 1_000;
        public const int QueryDays = 7;
        public const int RetentionDays = 7;
    }

    public static class IdGenerator
    {
        public const int SegmentDurationMs = 10_000;
        public const int SegmentSize = 131_072; // 2^17
    }

    public static class Lock
    {
        public const int DefaultTimeoutMs = 5_000;
        public const int RetryIntervalMs = 100;
    }

    public static class Pool
    {
        public const int TimeoutMs = 5_000;
        public const int MaxRetries = 3;
        public const int RetryDelayMs = 100;
    }

    public static class FileStorage
    {
        public const long MaxFileSizeBytes = 10_485_760; // 10MB
        public const long AvatarMaxSizeBytes = 2_097_152; // 2MB
    }
}
