using Vortex.Repositories;
using Vortex.Entities;
using Vortex.Core;
using Microsoft.Extensions.Logging;

namespace Vortex.Services;

public interface IMessageService
{
    Task<(SendMessageResult? Result, Error? Error)> SendMessageAsync(
        User currentUser, string targetPublicId, string typeField, string content);
    
    Task<(bool Success, Message? Message, Error? Error)> GetMessageAsync(
        long msgId, long msgTimestamp);
    
    Task<(bool Success, IEnumerable<Message> Messages, Error? Error)> GetConversationMessagesAsync(
        long convId, DateOnly date, int pageSize, int offset);
    
    Task<(bool Success, Error? Error)> RecallMessageAsync(
        long msgId, long msgTimestamp, long userId);
}

public class MessageService : IMessageService
{
    private readonly IMessageRepository _messageRepository;
    private readonly IUserRepository _userRepository;
    private readonly IGroupRepository _groupRepository;
    private readonly IOutboxService _outboxService;
    private readonly IdGenerator _idGenerator;
    private readonly ILogger<MessageService> _logger;
    
    private const long EpochTime = 1_767_225_600_000;
    
    public MessageService(
        IMessageRepository messageRepository,
        IUserRepository userRepository,
        IGroupRepository groupRepository,
        IOutboxService outboxService,
        IdGenerator idGenerator,
        ILogger<MessageService> logger)
    {
        _messageRepository = messageRepository ?? throw new ArgumentNullException(nameof(messageRepository));
        _userRepository = userRepository ?? throw new ArgumentNullException(nameof(userRepository));
        _groupRepository = groupRepository ?? throw new ArgumentNullException(nameof(groupRepository));
        _outboxService = outboxService ?? throw new ArgumentNullException(nameof(outboxService));
        _idGenerator = idGenerator ?? throw new ArgumentNullException(nameof(idGenerator));
        _logger = logger ?? throw new ArgumentNullException(nameof(logger));
    }
    
    public async Task<(SendMessageResult? Result, Error? Error)> SendMessageAsync(
        User currentUser, string targetPublicId, string typeField, string content)
    {
        try
        {
            var uid = currentUser.Id;
            var publicId = currentUser.PublicId;

            var convIdResult = await GenerateConversationIdAsync(typeField, uid, targetPublicId);
            if (convIdResult.Error is not null)
            {
                return (null, convIdResult.Error);
            }

            var convId = convIdResult.ConvId!.Value;
            
            var permError = await EnsureSessionPermissionAsync(uid, convId, typeField, targetPublicId);
            if (permError is not null)
            {
                return (null, permError);
            }

            var (msgIdResult, idError) = await _idGenerator.GenerateIdAsync();
            if (idError is not null)
            {
                return (null, idError);
            }

            var ts = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds() - EpochTime;

            var message = new OutboxMessage
            {
                MsgId = msgIdResult.ToString(),
                ConvId = convId.ToString(),
                FromUid = uid,
                Content = content,
                Ts = ts,
                IsRecalled = 0
            };

            var (outboxMessage, outboxError) = await _outboxService.SendAsync(message);
            if (outboxError is not null)
            {
                return (null, outboxError);
            }

            var result = new SendMessageResult
            {
                MsgId = message.MsgId,
                ConvId = convId,
                FromUid = uid,
                Content = content,
                Ts = ts,
                IsRecalled = 0
            };

            return (result, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "发送消息时发生错误");
            return (null, Error.InternalError("发送消息失败"));
        }
    }
    
    public async Task<(bool Success, Message? Message, Error? Error)> GetMessageAsync(
        long msgId, long msgTimestamp)
    {
        try
        {
            var date = DateOnly.FromDateTime(DateTimeOffset.FromUnixTimeMilliseconds(msgTimestamp).UtcDateTime);
            var tableName = $"messages_{date:yyyyMMdd}";

            var message = await _messageRepository.GetMessageAsync(tableName, msgId);
            
            if (message is null)
            {
                return (false, null, Error.NotFound("消息不存在"));
            }

            return (true, message, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取消息时发生错误");
            return (false, null, Error.InternalError("获取消息失败"));
        }
    }
    
    public async Task<(bool Success, IEnumerable<Message> Messages, Error? Error)> GetConversationMessagesAsync(
        long convId, DateOnly date, int pageSize, int offset)
    {
        try
        {
            var tableName = $"messages_{date:yyyyMMdd}";
            
            var messages = await _messageRepository.GetConversationMessagesAsync(tableName, convId, pageSize, offset);
            
            return (true, messages, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取会话消息时发生错误");
            return (false, Enumerable.Empty<Message>(), Error.InternalError("获取会话消息失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> RecallMessageAsync(
        long msgId, long msgTimestamp, long userId)
    {
        try
        {
            var date = DateOnly.FromDateTime(DateTimeOffset.FromUnixTimeMilliseconds(msgTimestamp).UtcDateTime);
            var tableName = $"messages_{date:yyyyMMdd}";

            var message = await _messageRepository.GetMessageAsync(tableName, msgId);
            if (message is null)
            {
                return (false, Error.NotFound("消息不存在"));
            }

            if (message.FromUid != userId)
            {
                return (false, Error.Forbidden("无权撤回该消息"));
            }

            message.IsRecalled = 1;
            var updated = await _messageRepository.UpdateMessageAsync(tableName, message);
            
            if (updated <= 0)
            {
                return (false, Error.InternalError("撤回消息失败"));
            }

            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "撤回消息时发生错误");
            return (false, Error.InternalError("撤回消息失败"));
        }
    }
    
    private async Task<(long? ConvId, Error? Error)> GenerateConversationIdAsync(
        string typeField, long uid, string targetPublicId)
    {
        var timestamp = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
        var convId = (timestamp << 20) | (uid & 0xFFFFF);
        
        return (convId, null);
    }
    
    private async Task<Error?> EnsureSessionPermissionAsync(
        long uid, long convId, string typeField, string targetPublicId)
    {
        if (typeField == "user")
        {
            var targetUser = await _userRepository.GetByPublicIdAsync(targetPublicId);
            if (targetUser is null)
            {
                return Error.NotFound("目标用户不存在");
            }
        }
        else if (typeField == "group")
        {
            var group = await _groupRepository.GetByIdAsync(long.Parse(targetPublicId));
            if (group is null)
            {
                return Error.NotFound("目标群组不存在");
            }
        }
        
        return null;
    }
}

public class SendMessageResult
{
    public string MsgId { get; set; } = string.Empty;
    public long ConvId { get; set; }
    public long FromUid { get; set; }
    public string Content { get; set; } = string.Empty;
    public long Ts { get; set; }
    public int IsRecalled { get; set; }
}
