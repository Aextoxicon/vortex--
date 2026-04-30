using Microsoft.AspNetCore.Mvc;
using Microsoft.AspNetCore.RateLimiting;
using Vortex.Entities;
using Vortex.Services;
using Vortex.Auth;

namespace Vortex.Controllers;

[Route("api/[controller]")]
public class AuthController : BaseController
{
    private readonly IUserService _userService;
    private readonly JwtService _jwtService;
    private readonly ILogger<AuthController> _logger;

    public AuthController(IUserService userService, JwtService jwtService, ILogger<AuthController> logger)
    {
        _userService = userService;
        _jwtService = jwtService;
        _logger = logger;
    }

    [HttpPost("register")]
    [EnableRateLimiting("register")]
    public async Task<IActionResult> Register([FromBody] RegisterRequest request)
    {
        var (userId, error) = await _userService.CreateUserAsync(
            request.Username,
            request.Password,
            request.Email);

        if (error != null)
        {
            return HandleError(error);
        }

        var (user, getUserError) = await _userService.GetUserByIdAsync(userId!.Value);
        if (getUserError != null)
        {
            return HandleError(getUserError);
        }

        var (token, tokenError) = _jwtService.GenerateToken(user!);
        if (tokenError != null)
        {
            return HandleError(tokenError);
        }

        return Created("", new
        {
            user!.PublicId,
            user.Username,
            user.Email,
            Token = token
        });
    }

    [HttpPost("login")]
    [EnableRateLimiting("login")]
    public async Task<IActionResult> Login([FromBody] LoginRequest request)
    {
        var (user, getUserError) = await _userService.GetUserByUsernameAsync(request.Username);
        if (getUserError != null)
        {
            return Unauthorized(new ErrorResponse("Invalid username or password"));
        }

        var (valid, validateError) = await _userService.ValidateCredentialsAsync(request.Username, request.Password);
        if (!valid || validateError != null)
        {
            return Unauthorized(new ErrorResponse("Invalid username or password"));
        }

        var (token, tokenError) = _jwtService.GenerateToken(user!);
        if (tokenError != null)
        {
            return HandleError(tokenError);
        }

        return Ok(new
        {
            Token = token,
            user!.PublicId,
            Message = "Login successful"
        });
    }

    [HttpGet("me")]
    [Microsoft.AspNetCore.Authorization.Authorize]
    public async Task<IActionResult> GetCurrentUser()
    {
        var userIdClaim = User.FindFirst(System.Security.Claims.ClaimTypes.NameIdentifier)?.Value;

        if (string.IsNullOrEmpty(userIdClaim) || !long.TryParse(userIdClaim, out var userId))
        {
            return Unauthorized(new ErrorResponse("Invalid token"));
        }

        var (user, error) = await _userService.GetUserByIdAsync(userId);

        if (error != null)
        {
            return HandleError(error);
        }

        return Ok(new { user = new { user!.Id, user.Username, user.Email, user.PublicId } });
    }
}

public record RegisterRequest(string Username, string? Email, string Password);
public record LoginRequest(string Username, string Password);
