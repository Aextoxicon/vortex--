using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;

namespace Vortex.Entities;

[Table("friend_requests")]
public class FriendRequest
{
    [Key]
    [DatabaseGenerated(DatabaseGeneratedOption.Identity)]
    [Column("id")]
    public long Id { get; set; }

    [Required]
    [Column("from_user_id")]
    public long FromUserId { get; set; }

    [Required]
    [Column("to_user_id")]
    public long ToUserId { get; set; }

    [Column("message")]
    [MaxLength(500)]
    public string? Message { get; set; }

    [Required]
    [Column("status")]
    [MaxLength(50)]
    public string Status { get; set; } = "pending";

    [Required]
    [Column("created_at")]
    public long CreatedAt { get; set; }

    [Column("updated_at")]
    public long UpdatedAt { get; set; }
}
