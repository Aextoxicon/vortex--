using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;

namespace Vortex.Entities;

[Table("users")]
public class User
{
    [Key]
    [DatabaseGenerated(DatabaseGeneratedOption.Identity)]
    [Column("id")]
    public long Id { get; set; }

    [Required]
    [Column("username")]
    [MaxLength(255)]
    public string Username { get; set; } = string.Empty;

    [Required]
    [Column("pwd_hash")]
    public string PwdHash { get; set; } = string.Empty;

    [Column("email")]
    [MaxLength(255)]
    public string? Email { get; set; }

    [Required]
    [Column("public_id")]
    [MaxLength(255)]
    public string PublicId { get; set; } = string.Empty;

    [Required]
    [Column("created_at")]
    public long CreatedAt { get; set; }

    [Column("updated_at")]
    public long UpdatedAt { get; set; }

    public ICollection<UserDevice> Devices { get; set; } = new List<UserDevice>();
}
