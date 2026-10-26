using System.Text;
using UnityEngine;

// Writes every item prefab's shared numbers to /config/items.tsv inside a
// game server: what valheim-save needs to put an item into a character file
// that the game would have made itself. It is a valheim-npc probe: compiled
// by that repo's tools/probe.sh and posted to its server's /probe endpoint,
// which `make items` here does when valheim-npc is checked out beside this
// repo. Read-only, it instantiates nothing.
//
// Durability is what the file stores, so the maximum at a quality is wanted:
// GetMaxDurability(q) is m_maxDurability + (q - 1) * m_durabilityPerLevel, and
// both parts are written so the Go side can compute it for any quality. An
// item that does not use durability still carries the numbers and the game
// ignores them. Stack, quality cap and teleportability are the limits the
// game applies on its own; writing past them is either clamped on load or
// shown as the item it is, so they are reported rather than enforced here.
public class Probe
{
    public static string Run()
    {
        var sb = new StringBuilder();
        sb.AppendLine("# prefab\ttype\tstack\tmaxQuality\tmaxDurability\tdurabilityPerLevel\tuseDurability\tteleportable\tweight\tname");
        int n = 0;
        foreach (var p in ZNetScene.instance.m_prefabs)
        {
            var d = p.GetComponent<ItemDrop>();
            if (d == null) continue;
            var s = d.m_itemData.m_shared;
            sb.AppendLine(string.Format("{0}\t{1}\t{2}\t{3}\t{4}\t{5}\t{6}\t{7}\t{8}\t{9}",
                p.name, s.m_itemType, s.m_maxStackSize, s.m_maxQuality, s.m_maxDurability, s.m_durabilityPerLevel,
                s.m_useDurability ? 1 : 0, s.m_teleportable ? 1 : 0, s.m_weight, s.m_name));
            n++;
        }
        System.IO.File.WriteAllText("/config/items.tsv", sb.ToString());
        return n + " items written to /config/items.tsv";
    }
}
