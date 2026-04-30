using System.Security.Claims;
using Vortex.Services;

namespace Vortex.Auth;

public class AuthMiddleware(RequestDelegate next, JwtService jwtService, IUserService userService)
{
    public async Task InvokeAsync(HttpContext context)
    {
        var authHeader = context.Request.Headers.Authorization.FirstOrDefault();

        if (string.IsNullOrEmpty(authHeader) || !authHeader.StartsWith("Bearer"))
        {
            await next(context);
            return;
        }

        var token = authHeader["Bearer ".Length..];

        var (principal, error) = jwtService.ValidateToken(token);

        if (principal is null || error is not null)
        {
            await WriteUnauthorizedResponse(context, "Unauthorized");
            return;
        }

        var userIdClaim = principal.FindFirst(ClaimTypes.NameIdentifier)?.Value;

        if (string.IsNullOrEmpty(userIdClaim) || !long.TryParse(userIdClaim, out var userId))
        {
            await WriteUnauthorizedResponse(context, "Invalid token claims");
            return;
        }

        var (user, userError) = await userService.GetUserByIdAsync(userId);

        if (user is null || userError is not null)
        {
            await WriteUnauthorizedResponse(context, "User not found");
            return;
        }

        context.User = principal;
        await next(context);
    }

    private static async Task WriteUnauthorizedResponse(HttpContext context, string message)
    {
        context.Response.StatusCode = 401;
        context.Response.ContentType = "application/json";
        await context.Response.WriteAsJsonAsync(new ErrorResponse(message), AppJsonContext.Default.ErrorResponse);
    }
}
