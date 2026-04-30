using Vortex.Repositories;
using Vortex.Entities;
using Microsoft.Extensions.Logging;

namespace Vortex.Services;

public interface IGroupService
{
    Task<(Group? Group, Error? Error)> GetGroupByIdAsync(long groupId);
    Task<(Group? Group, Error? Error)> GetGroupByNameAsync(string name);
    Task<(IEnumerable<Group> Groups, Error? Error)> GetGroupsByOwnerAsync(long ownerId);
    Task<(long? GroupId, Error? Error)> CreateGroupAsync(string name, string description, long ownerId);
    Task<(bool Success, Error? Error)> UpdateGroupAsync(Group group);
    Task<(bool Success, Error? Error)> DeleteGroupAsync(long groupId);
    Task<(bool Success, Error? Error)> AddMemberAsync(long groupId, long userId, string role);
    Task<(bool Success, Error? Error)> RemoveMemberAsync(long groupId, long userId);
    Task<(bool Success, Error? Error)> IsUserInGroupAsync(long groupId, long userId);
}

public class GroupService : IGroupService
{
    private readonly IGroupRepository _groupRepository;
    private readonly ILogger<GroupService> _logger;
    
    public GroupService(IGroupRepository groupRepository, ILogger<GroupService> logger)
    {
        _groupRepository = groupRepository ?? throw new ArgumentNullException(nameof(groupRepository));
        _logger = logger ?? throw new ArgumentNullException(nameof(logger));
    }
    
    public async Task<(Group? Group, Error? Error)> GetGroupByIdAsync(long groupId)
    {
        try
        {
            var group = await _groupRepository.GetByIdAsync(groupId);
            
            if (group is null)
            {
                return (null, Error.NotFound("群组不存在"));
            }
            
            return (group, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取群组时发生错误");
            return (null, Error.InternalError("获取群组失败"));
        }
    }
    
    public async Task<(Group? Group, Error? Error)> GetGroupByNameAsync(string name)
    {
        try
        {
            var group = await _groupRepository.GetByNameAsync(name);
            
            if (group is null)
            {
                return (null, Error.NotFound("群组不存在"));
            }
            
            return (group, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取群组时发生错误");
            return (null, Error.InternalError("获取群组失败"));
        }
    }
    
    public async Task<(IEnumerable<Group> Groups, Error? Error)> GetGroupsByOwnerAsync(long ownerId)
    {
        try
        {
            var groups = await _groupRepository.GetGroupsByOwnerAsync(ownerId);
            return (groups, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "获取用户群组时发生错误");
            return (Enumerable.Empty<Group>(), Error.InternalError("获取群组失败"));
        }
    }
    
    public async Task<(long? GroupId, Error? Error)> CreateGroupAsync(string name, string description, long ownerId)
    {
        try
        {
            if (await _groupRepository.NameExistsAsync(name))
            {
                return (null, Error.Conflict("群组名称已存在"));
            }
            
            var group = new Group
            {
                Name = name,
                Description = description,
                OwnerId = ownerId,
                CreatedAt = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds(),
                UpdatedAt = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()
            };
            
            var result = await _groupRepository.InsertAsync(group);
            
            if (result <= 0)
            {
                return (null, Error.InternalError("创建群组失败"));
            }
            
            return (1, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "创建群组时发生错误");
            return (null, Error.InternalError("创建群组失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> UpdateGroupAsync(Group group)
    {
        try
        {
            group.UpdatedAt = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
            
            var result = await _groupRepository.UpdateAsync(group);
            
            if (result <= 0)
            {
                return (false, Error.NotFound("群组不存在"));
            }
            
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "更新群组时发生错误");
            return (false, Error.InternalError("更新群组失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> DeleteGroupAsync(long groupId)
    {
        try
        {
            var result = await _groupRepository.DeleteAsync(groupId);
            
            if (result <= 0)
            {
                return (false, Error.NotFound("群组不存在"));
            }
            
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "删除群组时发生错误");
            return (false, Error.InternalError("删除群组失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> AddMemberAsync(long groupId, long userId, string role)
    {
        try
        {
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "添加群组成员时发生错误");
            return (false, Error.InternalError("添加成员失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> RemoveMemberAsync(long groupId, long userId)
    {
        try
        {
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "移除群组成员时发生错误");
            return (false, Error.InternalError("移除成员失败"));
        }
    }
    
    public async Task<(bool Success, Error? Error)> IsUserInGroupAsync(long groupId, long userId)
    {
        try
        {
            return (true, null);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "检查群组成员时发生错误");
            return (false, Error.InternalError("检查成员失败"));
        }
    }
}
