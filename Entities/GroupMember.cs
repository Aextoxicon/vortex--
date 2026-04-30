using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;

namespace Vortex.Entities;

[Table("group_members")]
public class GroupMember
{
    [Key]
    [DatabaseGenerated(DatabaseGeneratedOption.Identity)]
    [Column("id")]
    public long Id { get; set; }

    [Required]
    [Column("group_id")]
    [MaxLength(255)]
    public string GroupId { get; set; } = string.Empty;

    [Required]
    [Column("uid")]
    public long Uid { get; set; }

    [Required]
    [Column("role")]
    [MaxLength(50)]
    public string Role { get; set; } = "member";

    [Required]
    [Column("joined_at")]
    public long JoinedAt { get; set; }
}
