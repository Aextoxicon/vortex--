namespace Vortex;

public class Error
{
    public string Code { get; set; } = string.Empty;
    public string Message { get; set; } = string.Empty;
    public object? Details { get; set; }

    public Error(string code, string message, object? details = null)
    {
        Code = code;
        Message = message;
        Details = details;
    }

    public static Error NotFound(string message = "Resource not found") =>
        new("not_found", message);

    public static Error Forbidden(string message = "Access denied") =>
        new("forbidden", message);

    public static Error Unauthorized(string message = "Unauthorized") =>
        new("unauthorized", message);

    public static Error InvalidCredentials(string message = "Invalid credentials") =>
        new("invalid_credentials", message);

    public static Error RateLimitExceeded(string message = "Rate limit exceeded") =>
        new("rate_limit_exceeded", message);

    public static Error InvalidInput(string message) =>
        new("invalid_input", message);

    public static Error InvalidType(string message) =>
        new("invalid_type", message);

    public static Error InvalidTargetId(string message = "Invalid target ID") =>
        new("invalid_target_id", message);

    public static Error SelfRequest(string message = "Cannot request self") =>
        new("self_request", message);

    public static Error AlreadyExists(string message = "Resource already exists") =>
        new("already_exists", message);

    public static Error Conflict(string message = "Resource conflict") =>
        new("conflict", message);

    public static Error NotMember(string message = "Not a group member") =>
        new("not_member", message);

    public static Error InvalidGroup(string message = "Group not found or deleted") =>
        new("invalid_group", message);

    public static Error IdGenerationFailed(string message = "ID generation failed") =>
        new("id_generation_failed", message);

    public static Error TokenGenerationFailed(string message = "Token generation failed") =>
        new("token_generation_failed", message);

    public static Error NotPending(string message = "Status is not pending") =>
        new("not_pending", message);

    public static Error InvalidAction(string message = "Invalid action") =>
        new("invalid_action", message);

    public static Error AutoAccepted(string message = "Auto-accepted reverse request") =>
        new("auto_accepted", message);

    public static Error InternalError(string message = "Internal error") =>
        new("internal_error", message);

    public static Error Changeset(string message, object? details = null) =>
        new("changeset", message, details);
}
