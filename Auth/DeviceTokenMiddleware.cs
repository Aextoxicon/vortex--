using System.Security.Claims;
using Vortex.Entities;
using Vortex.Services;

namespace Vortex.Auth;

public class DeviceTokenMiddleware(RequestDelegate next)
{
    public async Task InvokeAsync(HttpContext context, IUserService userService)
    {
        var userId = context.User.FindFirst(ClaimTypes.NameIdentifier)?.Value;
        var deviceToken = context.Request.Headers["X-Device-Token"].FirstOrDefault();

        if (!string.IsNullOrEmpty(userId) && !string.IsNullOrEmpty(deviceToken))
        {
            var (user, error) = await userService.GetUserByIdAsync(long.Parse(userId));

            if (user is null || error is not null)
            {
                await WriteUnauthorizedResponse(context, "User not found");
                return;
            }
        }

        await next(context);
    }

    private static async Task WriteUnauthorizedResponse(HttpContext context, string message)
    {
        context.Response.StatusCode = 401;
        context.Response.ContentType = "application/json";
        await context.Response.WriteAsJsonAsync(new ErrorResponse(message), AppJsonContext.Default.ErrorResponse);
    }
}
