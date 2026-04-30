namespace Vortex.Database;

public static class TableNameValidator
{
    public static bool IsValidMessageTableName(string tableName)
    {
        if (string.IsNullOrEmpty(tableName))
            return false;

        return tableName.StartsWith("messages_") &&
               tableName.Length == 17 &&
               long.TryParse(tableName[9..], out _);
    }
}
