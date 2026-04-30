using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;

namespace Vortex.Entities;

[Table("id_generator_state")]
public class IdGeneratorState
{
    [Key]
    [Column("id")]
    public long Id { get; set; }

    [Required]
    [Column("last_ts")]
    public long LastTs { get; set; }

    [Required]
    [Column("last_seq")]
    public long LastSeq { get; set; }
}
