using Vortex.Repositories;
using Vortex.Entities;
using Microsoft.Extensions.Logging;

namespace Vortex.Services;

public interface IOutboxService
{
    Task<(OutboxMessage? Message, Error? Error)> SendAsync(OutboxMessage message);
    Task<(IEnumerable<OutboxMessage> Messages, Error? Error)> GetPendingMessagesAsync(int limit = 100);
    Task<(bool Success, Error? Error)> MarkAsProcessedAsync(long messageId);
    Task<(bool Success, Error? Error)> MarkAsFailedAsync(long messageId, string errorMessage);
    Task<(bool Success, Error? Error)> RetryFailedMessagesAsync(int limit = 100);
}

public class OutboxService : IOutboxService, IHostedService
{
    private readonly IOutboxMessageRepository _outboxMessageRepository;
    private readonly ILogger<OutboxService> _logger;
    private Timer? _timer;
    
    public OutboxService(IOutboxMessageRepository outboxMessageRepository, ILogger<OutboxService> logger)
    {
        _outboxMessageRepository = outboxMessageRepository ?? throw new ArgumentNullException(nameof(outboxMessageRepository));
        _logger = logger ?? throw new ArgumentNullException(nameof(logger));
    }
    
    public async Task<(OutboxMessage? Message, Error? Error)> SendAsync(OutboxMessage message)
    {
        try
        {
            message.Status = "pending";
            message.CreatedAt = DateTime.UtcNow;
            message.UpdatedAt = DateTime.UtcNow;
            
            var result = await _outboxMessageRepository.InsertAsync(message);
            
            if (result <= 0)
            {
                return (null, Error.InternalError("发送消息到队列失败"));
            }
            
            _logger.LogInformation("消息已添加到队列: {MsgId}", message.MsgId);
            return (message, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "发送消息到队列时发生错误");
            return (null, Error.InternalError("发送消息到队列失败"));
        }
    }
    
    public async Task<(IEnumerable<OutboxMessage> Messages, Error? Error)> GetPendingMessagesAsync(int limit = 100)
    {
        try
        {
            var messages = await _outboxMessageRepository.GetPendingMessagesAsync(limit);
            return (messages, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取待处理消息时发生错误");
            return (Enumerable.Empty<OutboxMessage>(), Error.InternalError("获取待处理消息失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> MarkAsProcessedAsync(long messageId)
    {
        try
        {
            var result = await _outboxMessageRepository.MarkAsProcessedAsync(messageId);
            
            if (result <= 0)
            {
                return (false, Error.NotFound("消息不存在"));
            }
            
            _logger.LogInformation("消息已标记为已处理: {MessageId}", messageId);
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "标记消息为已处理时发生错误");
            return (false, Error.InternalError("标记消息为已处理失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> MarkAsFailedAsync(long messageId, string errorMessage)
    {
        try
        {
            var result = await _outboxMessageRepository.MarkAsFailedAsync(messageId, errorMessage);
            
            if (result <= 0)
            {
                return (false, Error.NotFound("消息不存在"));
            }
            
            _logger.LogWarning("消息标记为失败: {MessageId}, 错误: {ErrorMessage}", messageId, errorMessage);
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "标记消息为失败时发生错误");
            return (false, Error.InternalError("标记消息为失败失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> RetryFailedMessagesAsync(int limit = 100)
    {
        try
        {
            var failedMessages = await _outboxMessageRepository.GetFailedMessagesAsync(limit);
            
            foreach (var message in failedMessages)
            {
                message.Status = "pending";
                message.UpdatedAt = DateTime.UtcNow;
                
                await _outboxMessageRepository.UpdateAsync(message);
            }
            
            _logger.LogInformation("已重试 {Count} 条失败消息", failedMessages.Count());
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "重试失败消息时发生错误");
            return (false, Error.InternalError("重试失败消息失败"));
        }
    }
    
    public Task StartAsync(CancellationToken cancellationToken)
    {
        _logger.LogInformation("Outbox服务启动");
        _timer = new Timer(ProcessMessages, null, TimeSpan.Zero, TimeSpan.FromSeconds(30));
        return Task.CompletedTask;
    }
    
    public Task StopAsync(CancellationToken cancellationToken)
    {
        _logger.LogInformation("Outbox服务停止");
        _timer?.Dispose();
        return Task.CompletedTask;
    }
    
    private async void ProcessMessages(object? state)
    {
        try
        {
            var (messages, error) = await GetPendingMessagesAsync(50);
            
            if (error is not null)
            {
                _logger.LogError("获取待处理消息失败: {Error}", error.Message);
                return;
            }
            
            foreach (var message in messages)
            {
                try
                {
                    await MarkAsProcessedAsync(message.Id);
                    _logger.LogInformation("消息处理成功: {MsgId}", message.MsgId);
                }
                catch (Exception ex)
                {
                    _logger.LogError(ex, "处理消息失败: {MsgId}", message.MsgId);
                    await MarkAsFailedAsync(message.Id, ex.Message);
                }
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "处理消息队列时发生错误");
        }
    }
}
