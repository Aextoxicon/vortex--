using Microsoft.AspNetCore.Mvc;
using System.Security.Claims;
using Vortex.Services;

namespace Vortex.Controllers;

[ApiController]
public abstract class BaseController : ControllerBase
{
    protected long? GetCurrentUserId()
    {
        var id = User.FindFirst(ClaimTypes.NameIdentifier)?.Value;
        return long.TryParse(id, out var userId) ? userId : null;
    }

    protected string? GetCurrentPublicId() =>
        User.FindFirst("public_id")?.Value;

    protected IActionResult HandleError(Error error) => error.Code switch
    {
        "not_found" => NotFound(new ErrorResponse(error.Message)),
        "forbidden" => StatusCode(403, new ErrorResponse(error.Message)),
        "unauthorized" or "invalid_credentials" => Unauthorized(new ErrorResponse(error.Message)),
        "already_exists" or "conflict" => Conflict(new ErrorResponse(error.Message)),
        "not_member" => StatusCode(403, new ErrorResponse(error.Message)),
        "self_request" => BadRequest(new ErrorResponse(error.Message)),
        "not_pending" => BadRequest(new ErrorResponse(error.Message)),
        "invalid_action" => BadRequest(new ErrorResponse(error.Message)),
        "invalid_type" or "invalid_input" or "invalid_target_id" or "invalid_group_format"
            => BadRequest(new ErrorResponse(error.Message)),
        "changeset" => UnprocessableEntity(new ErrorResponse(error.Message)),
        _ => StatusCode(500, new ErrorResponse(error.Message))
    };
}
