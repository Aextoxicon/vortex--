using System.Text.Json;
using Vortex.Core;
using Vortex.Entities;
using Vortex.Services;

namespace Vortex;

public class SystemNotification(IdGenerator idGenerator, IOutboxService outboxService, ILogger<SystemNotification> logger)
{
    private const long EpochTime = 1_767_225_600_000;

    public async Task<(long? MsgId, Error? Error)> SendToUserAsync(
        long uid,
        string notificationType,
        Dictionary<string, object> data)
    {
        var convId = $"system_{uid}";

        var notificationPayload = new NotificationPayload("system_notification", notificationType, data);
        var content = JsonSerializer.Serialize(notificationPayload, AppJsonContext.Default.NotificationPayload);

        var (msgIdResult, idError) = await idGenerator.GenerateIdAsync();
        if (idError is not null)
        {
            return (null, idError);
        }

        var ts = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds() - EpochTime;

        var message = new OutboxMessage
        {
            MsgId = msgIdResult.ToString(),
            ConvId = convId,
            FromUid = 0,
            Content = content,
            Ts = ts,
            IsRecalled = 0
        };

        try
        {
            var (_, sendError) = await outboxService.SendAsync(message);
            if (sendError is not null)
            {
                return (null, sendError);
            }
            return (msgIdResult, null);
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to enqueue system notification");
            return (null, Error.InternalError("Failed to send notification"));
        }
    }

    public async Task<(long? MsgId, Error? Error)> SendFriendRequestAsync(
        long receiverUid,
        long senderUid,
        string requestId)
    {
        return await SendToUserAsync(receiverUid, "friend_request", new Dictionary<string, object>
        {
            ["request_id"] = requestId,
            ["sender_uid"] = senderUid,
            ["action"] = "received"
        });
    }

    public async Task<(long? MsgId, Error? Error)> SendFriendRequestAcceptedAsync(
        long senderUid,
        long receiverUid,
        string requestId)
    {
        return await SendToUserAsync(senderUid, "friend_request", new Dictionary<string, object>
        {
            ["request_id"] = requestId,
            ["receiver_uid"] = receiverUid,
            ["action"] = "accepted"
        });
    }

    public async Task<(long? MsgId, Error? Error)> SendGroupInviteAsync(
        long uid,
        string groupId,
        string groupName,
        long inviterUid)
    {
        return await SendToUserAsync(uid, "group_invite", new Dictionary<string, object>
        {
            ["group_id"] = groupId,
            ["group_name"] = groupName,
            ["inviter_uid"] = inviterUid,
            ["action"] = "invited"
        });
    }

    public async Task<(long? MsgId, Error? Error)> SendGroupPermissionChangeAsync(
        long uid,
        string groupId,
        string groupName,
        string changeType,
        Dictionary<string, object>? data = null)
    {
        var notificationData = new Dictionary<string, object>
        {
            ["group_id"] = groupId,
            ["group_name"] = groupName,
            ["change_type"] = changeType,
            ["data"] = data ?? new Dictionary<string, object>()
        };

        return await SendToUserAsync(uid, "group_permission_change", notificationData);
    }
}
