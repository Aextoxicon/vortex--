using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;
using Vortex.Entities;
using Vortex.Services;

namespace Vortex.Controllers;

[Route("api/[controller]")]
[Authorize]
public class GroupsController : BaseController
{
    private readonly IGroupService _groupService;
    private readonly IUserService _userService;
    private readonly ILogger<GroupsController> _logger;

    public GroupsController(IGroupService groupService, IUserService userService, ILogger<GroupsController> logger)
    {
        _groupService = groupService;
        _userService = userService;
        _logger = logger;
    }

    [HttpPost]
    public async Task<IActionResult> Create([FromBody] CreateGroupRequest request)
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var (groupId, error) = await _groupService.CreateGroupAsync(request.Name, request.Description ?? "", userId.Value);

        if (error is not null)
        {
            return HandleError(error);
        }

        await _groupService.AddMemberAsync(groupId!.Value, userId.Value, "owner");

        return Created("", new
        {
            id = groupId,
            name = request.Name,
            owner_public_id = GetCurrentPublicId()
        });
    }

    [HttpGet("{id}")]
    public async Task<IActionResult> GetGroup(long id)
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var (group, error) = await _groupService.GetGroupByIdAsync(id);

        if (error is not null)
        {
            return HandleError(error);
        }

        return Ok(new
        {
            id = group!.Id,
            group.Name,
            owner_id = group.OwnerId
        });
    }

    [HttpPut("{id}")]
    public async Task<IActionResult> Update(long id, [FromBody] UpdateGroupRequest request)
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var (group, getError) = await _groupService.GetGroupByIdAsync(id);
        if (getError is not null)
        {
            return HandleError(getError);
        }

        if (group!.OwnerId != userId.Value)
        {
            return StatusCode(403, new ErrorResponse("Only group owner can update group"));
        }

        if (!string.IsNullOrEmpty(request.Name))
        {
            group.Name = request.Name;
        }

        var (success, updateError) = await _groupService.UpdateGroupAsync(group);

        return updateError is not null
            ? HandleError(updateError)
            : Ok(new { id = group.Id, group.Name });
    }

    [HttpDelete("{id}")]
    public async Task<IActionResult> Delete(long id)
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var (group, getError) = await _groupService.GetGroupByIdAsync(id);
        if (getError is not null)
        {
            return HandleError(getError);
        }

        if (group!.OwnerId != userId.Value)
        {
            return StatusCode(403, new ErrorResponse("Only group owner can delete group"));
        }

        var (success, deleteError) = await _groupService.DeleteGroupAsync(id);

        return deleteError is not null
            ? HandleError(deleteError)
            : NoContent();
    }

    [HttpPost("{id}/join")]
    public async Task<IActionResult> Join(long id)
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var (success, error) = await _groupService.AddMemberAsync(id, userId.Value, "member");

        return error is not null
            ? HandleError(error)
            : Ok(new { message = "Successfully joined group" });
    }

    [HttpPost("{id}/leave")]
    public async Task<IActionResult> Leave(long id)
    {
        var userId = GetCurrentUserId();
        if (userId is null) return Unauthorized(new ErrorResponse("Unauthorized"));

        var (success, error) = await _groupService.RemoveMemberAsync(id, userId.Value);

        return error is not null
            ? HandleError(error)
            : Ok(new { message = "Successfully left group" });
    }
}

public class CreateGroupRequest
{
    public string Name { get; set; } = string.Empty;
    public string? Description { get; set; }
}

public class UpdateGroupRequest
{
    public string? Name { get; set; }
}
