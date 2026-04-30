using Vortex.Repositories;
using Vortex.Entities;
using Microsoft.Extensions.Logging;

namespace Vortex.Services;

public interface IUserService
{
    Task<(User? User, Error? Error)> GetUserByIdAsync(long userId);
    Task<(User? User, Error? Error)> GetUserByUsernameAsync(string username);
    Task<(User? User, Error? Error)> GetUserByPublicIdAsync(string publicId);
    Task<(long? UserId, Error? Error)> CreateUserAsync(string username, string password, string? email = null);
    Task<(bool Success, Error? Error)> UpdateUserAsync(User user);
    Task<(bool Success, Error? Error)> DeleteUserAsync(long userId);
    Task<(bool Success, Error? Error)> ValidateCredentialsAsync(string username, string password);
}

public class UserService : IUserService
{
    private readonly IUserRepository _userRepository;
    private readonly ILogger<UserService> _logger;
    
    public UserService(IUserRepository userRepository, ILogger<UserService> logger)
    {
        _userRepository = userRepository ?? throw new ArgumentNullException(nameof(userRepository));
        _logger = logger ?? throw new ArgumentNullException(nameof(logger));
    }
    
    public async Task<(User? User, Error? Error)> GetUserByIdAsync(long userId)
    {
        try
        {
            var user = await _userRepository.GetByIdAsync(userId);
            
            if (user is null)
            {
                return (null, Error.NotFound("用户不存在"));
            }
            
            return (user, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取用户时发生错误");
            return (null, Error.InternalError("获取用户失败"));
        }
    }
    
    public async Task<(User? User, Error? Error)> GetUserByUsernameAsync(string username)
    {
        try
        {
            var user = await _userRepository.GetByUsernameAsync(username);
            
            if (user is null)
            {
                return (null, Error.NotFound("用户不存在"));
            }
            
            return (user, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取用户时发生错误");
            return (null, Error.InternalError("获取用户失败"));
        }
    }
    
    public async Task<(User? User, Error? Error)> GetUserByPublicIdAsync(string publicId)
    {
        try
        {
            var user = await _userRepository.GetByPublicIdAsync(publicId);
            
            if (user is null)
            {
                return (null, Error.NotFound("用户不存在"));
            }
            
            return (user, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取用户时发生错误");
            return (null, Error.InternalError("获取用户失败"));
        }
    }
    
    public async Task<(long? UserId, Error? Error)> CreateUserAsync(string username, string password, string? email = null)
    {
        try
        {
            if (await _userRepository.UsernameExistsAsync(username))
            {
                return (null, Error.Conflict("用户名已存在"));
            }
            
            if (!string.IsNullOrEmpty(email) && await _userRepository.EmailExistsAsync(email))
            {
                return (null, Error.Conflict("邮箱已存在"));
            }
            
            var pwdHash = BCrypt.Net.BCrypt.HashPassword(password);
            var publicId = GeneratePublicId();
            
            var user = new User
            {
                Username = username,
                PwdHash = pwdHash,
                Email = email,
                PublicId = publicId,
                CreatedAt = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds(),
                UpdatedAt = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()
            };
            
            var result = await _userRepository.InsertAsync(user);
            
            if (result <= 0)
            {
                return (null, Error.InternalError("创建用户失败"));
            }
            
            return (1, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "创建用户时发生错误");
            return (null, Error.InternalError("创建用户失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> UpdateUserAsync(User user)
    {
        try
        {
            user.UpdatedAt = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
            
            var result = await _userRepository.UpdateAsync(user);
            
            if (result <= 0)
            {
                return (false, Error.NotFound("用户不存在"));
            }
            
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "更新用户时发生错误");
            return (false, Error.InternalError("更新用户失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> DeleteUserAsync(long userId)
    {
        try
        {
            var result = await _userRepository.DeleteAsync(userId);
            
            if (result <= 0)
            {
                return (false, Error.NotFound("用户不存在"));
            }
            
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "删除用户时发生错误");
            return (false, Error.InternalError("删除用户失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> ValidateCredentialsAsync(string username, string password)
    {
        try
        {
            var user = await _userRepository.GetByUsernameAsync(username);
            
            if (user is null)
            {
                return (false, Error.NotFound("用户不存在"));
            }
            
            if (!BCrypt.Net.BCrypt.Verify(password, user.PwdHash))
            {
                return (false, Error.Unauthorized("密码错误"));
            }
            
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "验证凭证时发生错误");
            return (false, Error.InternalError("验证凭证失败"));
        }
    }
    
    private string GeneratePublicId()
    {
        return Guid.NewGuid().ToString("N").Substring(0, 16);
    }
}
