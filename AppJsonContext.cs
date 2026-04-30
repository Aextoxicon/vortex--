using System.Text.Json.Serialization;
using Vortex.Entities;
using Vortex.Database;

namespace Vortex;

[JsonSerializable(typeof(object))]
[JsonSerializable(typeof(string))]
[JsonSerializable(typeof(int))]
[JsonSerializable(typeof(long))]
[JsonSerializable(typeof(bool))]
[JsonSerializable(typeof(IEnumerable<string>))]
[JsonSerializable(typeof(IEnumerable<int>))]
[JsonSerializable(typeof(IEnumerable<long>))]
[JsonSerializable(typeof(Dictionary<string, string>))]
[JsonSerializable(typeof(Dictionary<string, object>))]
[JsonSerializable(typeof(User))]
[JsonSerializable(typeof(IEnumerable<User>))]
[JsonSerializable(typeof(Message))]
[JsonSerializable(typeof(IEnumerable<Message>))]
[JsonSerializable(typeof(Group))]
[JsonSerializable(typeof(IEnumerable<Group>))]
[JsonSerializable(typeof(FriendRequest))]
[JsonSerializable(typeof(IEnumerable<FriendRequest>))]
[JsonSerializable(typeof(OutboxMessage))]
[JsonSerializable(typeof(IEnumerable<OutboxMessage>))]
[JsonSerializable(typeof(UserDevice))]
[JsonSerializable(typeof(IEnumerable<UserDevice>))]
[JsonSerializable(typeof(IdGeneratorState))]
[JsonSerializable(typeof(IEnumerable<IdGeneratorState>))]
[JsonSerializable(typeof(GroupMember))]
[JsonSerializable(typeof(IEnumerable<GroupMember>))]
[JsonSerializable(typeof(ConversationParticipant))]
[JsonSerializable(typeof(IEnumerable<ConversationParticipant>))]
[JsonSerializable(typeof(DatabaseStats))]
[JsonSerializable(typeof(SuccessResponse))]
[JsonSerializable(typeof(ErrorResponse))]
[JsonSerializable(typeof(LoginRequest))]
[JsonSerializable(typeof(RegisterRequest))]
[JsonSerializable(typeof(AuthResponse))]
[JsonSerializable(typeof(SendMessageRequest))]
[JsonSerializable(typeof(SendMessageResponse))]
[JsonSerializable(typeof(RecallMessageRequest))]
[JsonSerializable(typeof(CreateGroupRequest))]
[JsonSerializable(typeof(UpdateGroupRequest))]
[JsonSerializable(typeof(AddGroupMemberRequest))]
[JsonSerializable(typeof(SendFriendRequestRequest))]
[JsonSerializable(typeof(HandleFriendRequestRequest))]
[JsonSerializable(typeof(ConversationMessageResponse))]
[JsonSerializable(typeof(RateLimitResponse))]
[JsonSerializable(typeof(ImageContent))]
[JsonSerializable(typeof(PaginatedResult<Message>))]
[JsonSerializable(typeof(NotificationPayload))]
public partial class AppJsonContext : JsonSerializerContext
{
}

public record DatabaseStats(
    int user_count,
    int group_count,
    int friend_request_count);

public record SuccessResponse(bool success, string message);

public record ErrorResponse(string error);

public record LoginRequest(string username, string password);

public record RegisterRequest(string username, string password, string email);

public record AuthResponse(string token, User user);

public record SendMessageRequest(string targetPublicId, string type, string content);

public record SendMessageResponse(string msgId, long convId, long fromUid, string content, long ts);

public record RecallMessageRequest(long msgId, long msgTimestamp);

public record CreateGroupRequest(string name, string description);

public record UpdateGroupRequest(string name, string description);

public record AddGroupMemberRequest(string userPublicId);

public record SendFriendRequestRequest(string targetPublicId);

public record HandleFriendRequestRequest(long requestId, bool accept);

public record ConversationMessageResponse(IEnumerable<Message> messages);

public record RateLimitResponse(string error, string message, int retryAfterSeconds);

public record ImageContent(string type, string url, string text);

public record NotificationPayload(string type, string notification_type, Dictionary<string, object> data);
