using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;
using Vortex.Entities;
using Vortex.Services;

namespace Vortex.Controllers;

[Route("api/[controller]")]
[Authorize]
public class MessagesController : BaseController
{
    private readonly IMessageService _messageService;
    private readonly ILogger<MessagesController> _logger;

    public MessagesController(IMessageService messageService, ILogger<MessagesController> logger)
    {
        _messageService = messageService;
        _logger = logger;
    }

    [HttpPost("send")]
    public async Task<IActionResult> Send([FromBody] SendMessageRequest request)
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var content = request.ContentType == "image"
            ? System.Text.Json.JsonSerializer.Serialize(new ImageContent("image", request.Content, request.Text ?? ""), AppJsonContext.Default.ImageContent)
            : request.Content;

        var user = new User { Id = userId.Value, PublicId = GetCurrentPublicId()! };

        var (result, error) = await _messageService.SendMessageAsync(
            user,
            request.TargetPublicId,
            request.Type,
            content);

        return error is not null ? HandleError(error) : Created("", result);
    }

    [HttpGet]
    public async Task<IActionResult> GetMessages([FromQuery] long convId, [FromQuery] DateOnly date, [FromQuery] int pageSize = 100, [FromQuery] int offset = 0)
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var (success, messages, error) = await _messageService.GetConversationMessagesAsync(convId, date, pageSize, offset);

        return error is not null
            ? HandleError(error)
            : Ok(new ConversationMessageResponse(messages));
    }

    [HttpPost("recall/{msgId}")]
    public async Task<IActionResult> Recall(long msgId, [FromQuery] long msgTimestamp)
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var (success, error) = await _messageService.RecallMessageAsync(msgId, msgTimestamp, userId.Value);

        return error is not null
            ? HandleError(error)
            : Ok(new SuccessResponse(success, "Message recalled successfully"));
    }
}

public class SendMessageRequest
{
    public string TargetPublicId { get; set; } = string.Empty;
    public string Type { get; set; } = "p";
    public string Content { get; set; } = string.Empty;
    public string? Text { get; set; }
    public string? ContentType { get; set; } = "text";
}
