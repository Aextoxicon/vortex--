using Vortex.Repositories;
using Vortex.Entities;
using Microsoft.Extensions.Logging;

namespace Vortex.Services;

public interface IFriendRequestService
{
    Task<(FriendRequest? Request, Error? Error)> GetFriendRequestByIdAsync(long requestId);
    Task<(FriendRequest? Request, Error? Error)> GetFriendRequestByUsersAsync(long fromUserId, long toUserId);
    Task<(IEnumerable<FriendRequest> Requests, Error? Error)> GetSentRequestsAsync(long fromUserId);
    Task<(IEnumerable<FriendRequest> Requests, Error? Error)> GetReceivedRequestsAsync(long toUserId);
    Task<(IEnumerable<FriendRequest> Requests, Error? Error)> GetPendingRequestsAsync(long userId);
    Task<(long? RequestId, Error? Error)> SendFriendRequestAsync(long fromUserId, long toUserId, string? message = null);
    Task<(bool Success, Error? Error)> AcceptFriendRequestAsync(long requestId, long userId);
    Task<(bool Success, Error? Error)> RejectFriendRequestAsync(long requestId, long userId);
    Task<(bool Success, Error? Error)> CancelFriendRequestAsync(long requestId, long fromUserId);
    Task<(bool Success, Error? Error)> DeleteFriendRequestAsync(long requestId);
}

public class FriendRequestService : IFriendRequestService
{
    private readonly IFriendRequestRepository _friendRequestRepository;
    private readonly IUserRepository _userRepository;
    private readonly ILogger<FriendRequestService> _logger;
    
    public FriendRequestService(
        IFriendRequestRepository friendRequestRepository,
        IUserRepository userRepository,
        ILogger<FriendRequestService> logger)
    {
        _friendRequestRepository = friendRequestRepository ?? throw new ArgumentNullException(nameof(friendRequestRepository));
        _userRepository = userRepository ?? throw new ArgumentNullException(nameof(userRepository));
        _logger = logger ?? throw new ArgumentNullException(nameof(logger));
    }
    
    public async Task<(FriendRequest? Request, Error? Error)> GetFriendRequestByIdAsync(long requestId)
    {
        try
        {
            var request = await _friendRequestRepository.GetByIdAsync(requestId);
            
            if (request is null)
            {
                return (null, Error.NotFound("好友请求不存在"));
            }
            
            return (request, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取好友请求时发生错误");
            return (null, Error.InternalError("获取好友请求失败"));
        }
    }
    
    public async Task<(FriendRequest? Request, Error? Error)> GetFriendRequestByUsersAsync(long fromUserId, long toUserId)
    {
        try
        {
            var request = await _friendRequestRepository.GetByUsersAsync(fromUserId, toUserId);
            
            if (request is null)
            {
                return (null, Error.NotFound("好友请求不存在"));
            }
            
            return (request, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取好友请求时发生错误");
            return (null, Error.InternalError("获取好友请求失败"));
        }
    }
    
    public async Task<(IEnumerable<FriendRequest> Requests, Error? Error)> GetSentRequestsAsync(long fromUserId)
    {
        try
        {
            var requests = await _friendRequestRepository.GetSentRequestsAsync(fromUserId);
            return (requests, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取发送的好友请求时发生错误");
            return (Enumerable.Empty<FriendRequest>(), Error.InternalError("获取好友请求失败"));
        }
    }
    
    public async Task<(IEnumerable<FriendRequest> Requests, Error? Error)> GetReceivedRequestsAsync(long toUserId)
    {
        try
        {
            var requests = await _friendRequestRepository.GetReceivedRequestsAsync(toUserId);
            return (requests, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取收到的好友请求时发生错误");
            return (Enumerable.Empty<FriendRequest>(), Error.InternalError("获取好友请求失败"));
        }
    }
    
    public async Task<(IEnumerable<FriendRequest> Requests, Error? Error)> GetPendingRequestsAsync(long userId)
    {
        try
        {
            var requests = await _friendRequestRepository.GetPendingRequestsAsync(userId);
            return (requests, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取待处理的好友请求时发生错误");
            return (Enumerable.Empty<FriendRequest>(), Error.InternalError("获取好友请求失败"));
        }
    }
    
    public async Task<(long? RequestId, Error? Error)> SendFriendRequestAsync(long fromUserId, long toUserId, string? message = null)
    {
        try
        {
            var toUser = await _userRepository.GetByIdAsync(toUserId);
            if (toUser is null)
            {
                return (null, Error.NotFound("目标用户不存在"));
            }
            
            var existingRequest = await _friendRequestRepository.GetByUsersAsync(fromUserId, toUserId);
            if (existingRequest is not null)
            {
                return (null, Error.Conflict("已向该用户发送过好友请求"));
            }
            
            var request = new FriendRequest
            {
                FromUserId = fromUserId,
                ToUserId = toUserId,
                Message = message,
                Status = "pending",
                CreatedAt = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds(),
                UpdatedAt = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()
            };
            
            var result = await _friendRequestRepository.InsertAsync(request);
            
            if (result <= 0)
            {
                return (null, Error.InternalError("发送好友请求失败"));
            }
            
            return (1, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "发送好友请求时发生错误");
            return (null, Error.InternalError("发送好友请求失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> AcceptFriendRequestAsync(long requestId, long userId)
    {
        try
        {
            var request = await _friendRequestRepository.GetByIdAsync(requestId);
            
            if (request is null)
            {
                return (false, Error.NotFound("好友请求不存在"));
            }
            
            if (request.ToUserId != userId)
            {
                return (false, Error.Forbidden("无权处理该好友请求"));
            }
            
            if (request.Status != "pending")
            {
                return (false, Error.Conflict("好友请求已处理"));
            }
            
            request.Status = "accepted";
            request.UpdatedAt = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
            
            var result = await _friendRequestRepository.UpdateAsync(request);
            
            if (result <= 0)
            {
                return (false, Error.InternalError("接受好友请求失败"));
            }
            
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "接受好友请求时发生错误");
            return (false, Error.InternalError("接受好友请求失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> RejectFriendRequestAsync(long requestId, long userId)
    {
        try
        {
            var request = await _friendRequestRepository.GetByIdAsync(requestId);
            
            if (request is null)
            {
                return (false, Error.NotFound("好友请求不存在"));
            }
            
            if (request.ToUserId != userId)
            {
                return (false, Error.Forbidden("无权处理该好友请求"));
            }
            
            if (request.Status != "pending")
            {
                return (false, Error.Conflict("好友请求已处理"));
            }
            
            request.Status = "rejected";
            request.UpdatedAt = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
            
            var result = await _friendRequestRepository.UpdateAsync(request);
            
            if (result <= 0)
            {
                return (false, Error.InternalError("拒绝好友请求失败"));
            }
            
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "拒绝好友请求时发生错误");
            return (false, Error.InternalError("拒绝好友请求失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> CancelFriendRequestAsync(long requestId, long fromUserId)
    {
        try
        {
            var request = await _friendRequestRepository.GetByIdAsync(requestId);
            
            if (request is null)
            {
                return (false, Error.NotFound("好友请求不存在"));
            }
            
            if (request.FromUserId != fromUserId)
            {
                return (false, Error.Forbidden("无权取消该好友请求"));
            }
            
            if (request.Status != "pending")
            {
                return (false, Error.Conflict("好友请求已处理"));
            }
            
            var result = await _friendRequestRepository.DeleteAsync(requestId);
            
            if (result <= 0)
            {
                return (false, Error.InternalError("取消好友请求失败"));
            }
            
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "取消好友请求时发生错误");
            return (false, Error.InternalError("取消好友请求失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> DeleteFriendRequestAsync(long requestId)
    {
        try
        {
            var result = await _friendRequestRepository.DeleteAsync(requestId);
            
            if (result <= 0)
            {
                return (false, Error.NotFound("好友请求不存在"));
            }
            
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "删除好友请求时发生错误");
            return (false, Error.InternalError("删除好友请求失败"));
        }
    }
}
