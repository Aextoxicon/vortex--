using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;
using Vortex.Entities;
using Vortex.Services;

namespace Vortex.Controllers;

[Route("api/[controller]")]
[Authorize]
public class FriendsController : BaseController
{
    private readonly IFriendRequestService _friendRequestService;
    private readonly IUserService _userService;
    private readonly ILogger<FriendsController> _logger;

    public FriendsController(IFriendRequestService friendRequestService, IUserService userService, ILogger<FriendsController> logger)
    {
        _friendRequestService = friendRequestService;
        _userService = userService;
        _logger = logger;
    }

    [HttpPost("request/{targetPublicId}")]
    public async Task<IActionResult> SendRequest(string targetPublicId)
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var (targetUser, getError) = await _userService.GetUserByPublicIdAsync(targetPublicId);
        if (getError is not null)
        {
            return NotFound(new ErrorResponse("Target user not found"));
        }

        var (requestId, error) = await _friendRequestService.SendFriendRequestAsync(userId.Value, targetUser!.Id);

        if (error is not null)
        {
            return HandleError(error);
        }

        return Created("", new
        {
            id = requestId,
            status = "pending",
            sender_public_id = GetCurrentPublicId(),
            receiver_public_id = targetPublicId
        });
    }

    [HttpGet("requests")]
    public async Task<IActionResult> GetRequests()
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var (sentRequests, sentError) = await _friendRequestService.GetSentRequestsAsync(userId.Value);
        var (receivedRequests, receivedError) = await _friendRequestService.GetReceivedRequestsAsync(userId.Value);

        if (sentError is not null || receivedError is not null)
        {
            return StatusCode(500, new ErrorResponse("Failed to get requests"));
        }

        return Ok(new
        {
            sent = sentRequests.Select(FormatRequest),
            received = receivedRequests.Select(FormatRequest)
        });
    }

    [HttpPost("request/{requestId}/accept")]
    public async Task<IActionResult> AcceptRequest(long requestId)
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var (success, error) = await _friendRequestService.AcceptFriendRequestAsync(requestId, userId.Value);

        if (error is not null)
        {
            return HandleError(error);
        }

        return Ok(new { message = "Friend request accepted" });
    }

    [HttpPost("request/{requestId}/reject")]
    public async Task<IActionResult> RejectRequest(long requestId)
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var (success, error) = await _friendRequestService.RejectFriendRequestAsync(requestId, userId.Value);

        if (error is not null)
        {
            return HandleError(error);
        }

        return Ok(new { message = "Friend request rejected" });
    }

    [HttpDelete("request/{requestId}")]
    public async Task<IActionResult> CancelRequest(long requestId)
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var (success, error) = await _friendRequestService.CancelFriendRequestAsync(requestId, userId.Value);

        if (error is not null)
        {
            return HandleError(error);
        }

        return NoContent();
    }

    private object FormatRequest(FriendRequest request) => new
    {
        id = request.Id,
        sender_id = request.FromUserId,
        receiver_id = request.ToUserId,
        status = request.Status,
        ts = request.CreatedAt
    };
}
