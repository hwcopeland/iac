/*
 * ZoneEditor — live zone inspection and editing for Source2Surf.
 *
 * Map is auto-detected via IModSharp.GetMapName() on every server activate.
 *
 * Admin commands (require "admin:zone" or "*" permission, or MAP_ADMIN_STEAMIDS):
 *   !zones [page]            - list zones for current map (8 per page)
 *   !zoneinfo <id>           - dump full zone details
 *   !showzones               - spawn in-world labels for every zone (auto-expire 120s)
 *   !pos1                    - mark corner-1 at your current position
 *   !pos2                    - mark corner-2 at your current position
 *   !addzone <type> [track] [seq] - create zone from pos1+pos2
 *   !editzone <id>           - load existing zone's corners into pos1/pos2
 *   !savezone                - write current pos1/pos2 back to the zone in !editzone
 *   !delzone <id>            - delete zone (requires !delzone <id> confirm)
 *   !tpzone <id>             - teleport to zone center
 *
 * Zone type keywords for !addzone:
 *   start / s         → 0  (map start)
 *   end / finish / e  → 1  (map end)
 *   stage / st        → 2  (!addzone stage 0 <seq>)
 *   bonusstart / bs   → 3  (!addzone bonusstart <track>)
 *   bonusend / be     → 4  (!addzone bonusend <track>)
 */

using System;
using System.Collections.Generic;
using System.Linq;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging;
using MySqlConnector;
using Sharp.Shared;
using Sharp.Shared.Enums;
using Sharp.Shared.GameEntities;
using Sharp.Shared.Listeners;
using Sharp.Shared.Managers;
using Sharp.Shared.Objects;
using Sharp.Shared.Types;
using Sharp.Shared.Units;

namespace Cs2Surf.ZoneEditor;

public sealed class ZoneEditor : IModSharpModule, IClientListener, IGameListener
{
    public string DisplayName   => "Cs2Surf.ZoneEditor";
    public string DisplayAuthor => "hwcopeland";

    private readonly ISharedSystem          _shared;
    private readonly ILogger<ZoneEditor>    _logger;
    private readonly IClientManager         _clientManager;

    private string _mysqlConnStr = "";
    private string _currentMap   = "";

    private readonly Dictionary<ulong, AdminState> _state       = [];
    private readonly HashSet<ulong>                _liveAlert   = [];
    private readonly HashSet<ulong>                _envAdminIds = [];

    // Spawned world-text display entities — killed on map change or next !showzones.
    private readonly List<IBaseEntity> _displayEntities = [];
    private bool _displayActive = false;
    private int  _displayGen    = 0;

    private List<ZoneRow> _zoneCache = [];

    private sealed class AdminState
    {
        public Vec3? Pos1;
        public Vec3? Pos2;
        public long? EditingId;
    }

    private readonly record struct Vec3(float X, float Y, float Z)
    {
        public override string ToString() => $"{X:F1} {Y:F1} {Z:F1}";

        public static Vec3 Min(Vec3 a, Vec3 b) => new(
            MathF.Min(a.X, b.X), MathF.Min(a.Y, b.Y), MathF.Min(a.Z, b.Z));
        public static Vec3 Max(Vec3 a, Vec3 b) => new(
            MathF.Max(a.X, b.X), MathF.Max(a.Y, b.Y), MathF.Max(a.Z, b.Z));
        public static Vec3 Center(Vec3 mins, Vec3 maxs) => new(
            (mins.X + maxs.X) / 2f, (mins.Y + maxs.Y) / 2f, (mins.Z + maxs.Z) / 2f);

        public bool Inside(Vec3 mins, Vec3 maxs) =>
            X >= mins.X && X <= maxs.X &&
            Y >= mins.Y && Y <= maxs.Y &&
            Z >= mins.Z && Z <= maxs.Z;

        public static Vec3? TryParse(string? s)
        {
            if (string.IsNullOrWhiteSpace(s)) return null;
            var parts = s.Trim().Split(' ', StringSplitOptions.RemoveEmptyEntries);
            if (parts.Length >= 3
                && float.TryParse(parts[0], out var x)
                && float.TryParse(parts[1], out var y)
                && float.TryParse(parts[2], out var z))
                return new Vec3(x, y, z);
            return null;
        }
    }

    private sealed class ZoneRow
    {
        public long   Id       { get; init; }
        public long   MapId    { get; init; }
        public int    Type     { get; init; }
        public short  Track    { get; init; }
        public short  Sequence { get; init; }
        public Vec3   Mins     { get; init; }
        public Vec3   Maxs     { get; init; }
        public Vec3   Center   { get; init; }

        public string TypeName => Type switch
        {
            0 => "Start",
            1 => "End",
            2 => $"Stage {Sequence}",
            3 => $"BonusStart T{Track}",
            4 => $"BonusEnd T{Track}",
            _ => $"Type{Type}",
        };

        public string DisplayColor => Type switch
        {
            0 => "0 230 0",
            1 => "230 0 0",
            2 => "230 230 0",
            3 => "0 180 230",
            4 => "0 100 200",
            _ => "200 200 200",
        };
    }

    public ZoneEditor(ISharedSystem sharedSystem,
                      string        dllPath,
                      string        sharpPath,
                      Version       version,
                      IConfiguration coreConfiguration,
                      bool          hotReload)
    {
        _shared        = sharedSystem;
        _logger        = sharedSystem.GetLoggerFactory().CreateLogger<ZoneEditor>();
        _clientManager = sharedSystem.GetClientManager();
    }

    public bool Init() => true;

    public void PostInit()
    {
        var envAdmins = Environment.GetEnvironmentVariable("MAP_ADMIN_STEAMIDS") ?? "";
        foreach (var part in envAdmins.Split(',', StringSplitOptions.RemoveEmptyEntries
                                                   | StringSplitOptions.TrimEntries))
            if (ulong.TryParse(part, out var id)) _envAdminIds.Add(id);

        var dbHost = Environment.GetEnvironmentVariable("MYSQL_HOST") ?? "";
        var dbPort = Environment.GetEnvironmentVariable("MYSQL_PORT") ?? "3306";
        var dbUser = Environment.GetEnvironmentVariable("MYSQL_USER") ?? "";
        var dbPass = Environment.GetEnvironmentVariable("MYSQL_PASS") ?? "";

        if (!string.IsNullOrEmpty(dbHost) && !string.IsNullOrEmpty(dbUser))
        {
            _mysqlConnStr = $"Server={dbHost};Port={dbPort};Database=source2surf;User ID={dbUser};Password={dbPass};";
            _logger.LogInformation("ZoneEditor: MySQL configured");
        }
        else
        {
            _logger.LogWarning("ZoneEditor: MYSQL_HOST/MYSQL_USER not set");
        }

        _clientManager.InstallClientListener(this);
        _shared.GetModSharp().InstallGameListener(this);
        ScheduleProximityTick();
        _logger.LogInformation("ZoneEditor loaded");
    }

    public void Shutdown()
    {
        _clientManager.RemoveClientListener(this);
        _shared.GetModSharp().RemoveGameListener(this);
    }

    int IClientListener.ListenerVersion  => IClientListener.ApiVersion;
    int IClientListener.ListenerPriority => 0;
    int IGameListener.ListenerVersion    => IGameListener.ApiVersion;
    int IGameListener.ListenerPriority   => 0;

    void IGameListener.OnResourcePrecache() { }

    void IGameListener.OnServerActivate()
    {
        _state.Clear();
        _liveAlert.Clear();
        _displayEntities.Clear();
        _displayActive = false;
        _zoneCache.Clear();

        // Auto-detect map name from the engine — no env var needed.
        _currentMap = _shared.GetModSharp().GetMapName() ?? "";
        _logger.LogInformation("ZoneEditor: map={Map}", _currentMap);

        RefreshZoneCache();
    }

    public void OnClientPutInServer(IGameClient client) { }

    public void OnClientDisconnecting(IGameClient client, NetworkDisconnectionReason reason)
    {
        if (client.IsFakeClient) return;
        var id = (ulong)client.SteamId;
        _state.Remove(id);
        _liveAlert.Remove(id);
    }

    // ─── Chat dispatch ─────────────────────────────────────────────────

    public ECommandAction OnClientSayCommand(IGameClient client, bool teamOnly, bool isCommand,
                                             string commandName, string message)
    {
        var raw = (message ?? "").TrimStart();
        if (raw.Length == 0 || "!./`".IndexOf(raw[0]) < 0 || client.IsFakeClient)
            return ECommandAction.Skipped;

        var body     = raw.Substring(1);
        var spaceIdx = body.IndexOf(' ');
        var cmd      = (spaceIdx < 0 ? body : body[..spaceIdx]).ToLowerInvariant();
        var arg      = (spaceIdx < 0 ? "" : body[(spaceIdx + 1)..]).Trim();

        return cmd switch
        {
            "zones"                    => CmdZones(client, arg),
            "zoneinfo"                 => CmdZoneInfo(client, arg),
            "showzones"                => CmdShowZones(client),
            "pos1"                     => CmdPos1(client),
            "pos2"                     => CmdPos2(client),
            "addzone"                  => CmdAddZone(client, arg),
            "editzone"                 => CmdEditZone(client, arg),
            "savezone"                 => CmdSaveZone(client),
            "delzone"                  => CmdDelZone(client, arg),
            "tpzone"                   => CmdTpZone(client, arg),
            _                          => ECommandAction.Skipped,
        };
    }

    // ─── !zones ────────────────────────────────────────────────────────

    private ECommandAction CmdZones(IGameClient client, string arg)
    {
        if (string.IsNullOrEmpty(_mysqlConnStr))
        {
            Reply(client, "\x07[zone] DB not configured.");
            return ECommandAction.Handled;
        }
        int page = 1;
        if (!string.IsNullOrEmpty(arg)) int.TryParse(arg, out page);
        if (page < 1) page = 1;

        const int perPage = 8;
        var zones = _zoneCache;
        var total = zones.Count;
        var pages = Math.Max(1, (total + perPage - 1) / perPage);
        if (page > pages) page = pages;

        Reply(client, $"\x04===== Zones: \x09{_currentMap} \x04({total}) pg {page}/{pages} =====");
        foreach (var z in zones.Skip((page - 1) * perPage).Take(perPage))
            Reply(client, $"  \x09#{z.Id} \x08| \x0B{z.TypeName} \x08| {z.Mins} → {z.Maxs}");
        if (total == 0) Reply(client, "\x08  (none — use !pos1 !pos2 !addzone to create)");
        Reply(client, "\x08  !showzones to render  !tpzone <id> to go there");
        return ECommandAction.Handled;
    }

    // ─── !zoneinfo ─────────────────────────────────────────────────────

    private ECommandAction CmdZoneInfo(IGameClient client, string arg)
    {
        if (!long.TryParse(arg, out var id))
        {
            Reply(client, "\x07[zone] Usage: !zoneinfo <id>");
            return ECommandAction.Handled;
        }
        var z = _zoneCache.FirstOrDefault(x => x.Id == id) ?? FetchZone(id);
        if (z is null) { Reply(client, $"\x07[zone] Zone {id} not found."); return ECommandAction.Handled; }

        Reply(client, $"\x04===== Zone {z.Id} ({z.TypeName}) =====");
        Reply(client, $"  \x09Mins: \x01{z.Mins}");
        Reply(client, $"  \x09Maxs: \x01{z.Maxs}");
        Reply(client, $"  \x09Center: \x01{z.Center}");
        Reply(client, $"  \x09Track={z.Track}  Seq={z.Sequence}");
        return ECommandAction.Handled;
    }

    // ─── !showzones ────────────────────────────────────────────────────

    private ECommandAction CmdShowZones(IGameClient client)
    {
        if (!RequireAdmin(client, "!showzones")) return ECommandAction.Handled;

        if (_zoneCache.Count == 0)
        {
            Reply(client, "\x07[zone] No zones for this map.");
            return ECommandAction.Handled;
        }

        _shared.GetModSharp().InvokeFrameAction(() =>
        {
            try
            {
                SpawnZoneDisplay();
                Reply(client, $"\x04[zone] Showing \x09{_zoneCache.Count} \x04zones. Labels expire in 120s — re-run !showzones to refresh.");
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "ZoneEditor: SpawnZoneDisplay failed");
                Reply(client, "\x07[zone] Failed to spawn zone display.");
            }
        });

        return ECommandAction.Handled;
    }

    private void SpawnZoneDisplay()
    {
        // Kill any previous batch so we don't accumulate stale entities.
        KillDisplayEntities();

        var gen = ++_displayGen;
        var em  = _shared.GetEntityManager();
        foreach (var z in _zoneCache)
        {
            // Center label — raised 30 units so it floats above floor level.
            var center = z.Center;
            SpawnLabel(em, new Vec3(center.X, center.Y, center.Z + 30f),
                       $"#{z.Id} {z.TypeName}", z.DisplayColor, 12f);

            // Mins corner
            SpawnLabel(em, new Vec3(z.Mins.X, z.Mins.Y, z.Mins.Z + 5f),
                       $"#{z.Id} MIN", "150 150 150", 6f);

            // Maxs corner
            SpawnLabel(em, new Vec3(z.Maxs.X, z.Maxs.Y, z.Maxs.Z + 5f),
                       $"#{z.Id} MAX", "150 150 150", 6f);
        }

        _displayActive = true;

        // Auto-expire after 120 s — only kill if this is still the active generation.
        _shared.GetModSharp().PushTimer(() =>
        {
            if (_displayGen == gen) KillDisplayEntities();
        }, 120f, GameTimerFlags.None);
    }

    private void SpawnLabel(IEntityManager em, Vec3 pos, string text, string color, float size)
    {
        try
        {
            var ent = em.SpawnEntitySync("point_worldtext",
                new Dictionary<string, KeyValuesVariantValueItem>
                {
                    ["origin"]      = $"{pos.X:F1} {pos.Y:F1} {pos.Z:F1}",
                    ["message"]     = text,
                    ["textsize"]    = size,
                    ["rendercolor"] = color,
                    ["enabled"]     = "1",
                });
            if (ent is not null)
                _displayEntities.Add(ent);
        }
        catch (Exception ex)
        {
            _logger.LogWarning(ex, "ZoneEditor: SpawnLabel failed for '{Text}'", text);
        }
    }

    private void KillDisplayEntities()
    {
        foreach (var e in _displayEntities)
        {
            try { e.Kill(); }
            catch { }
        }
        _displayEntities.Clear();
        _displayActive = false;
    }

    // ─── !pos1 / !pos2 ─────────────────────────────────────────────────

    private ECommandAction CmdPos1(IGameClient client)
    {
        if (!RequireAdmin(client, "!pos1")) return ECommandAction.Handled;
        var pos = GetPlayerPosition(client);
        if (pos is null) { Reply(client, "\x07[zone] Cannot read position."); return ECommandAction.Handled; }
        var st = GetOrCreate((ulong)client.SteamId);
        st.Pos1 = pos;
        Reply(client, $"\x04[zone] Pos1: \x09{pos}");
        if (st.Pos2 is not null) Reply(client, $"\x08         Pos2: {st.Pos2}");
        return ECommandAction.Handled;
    }

    private ECommandAction CmdPos2(IGameClient client)
    {
        if (!RequireAdmin(client, "!pos2")) return ECommandAction.Handled;
        var pos = GetPlayerPosition(client);
        if (pos is null) { Reply(client, "\x07[zone] Cannot read position."); return ECommandAction.Handled; }
        var st = GetOrCreate((ulong)client.SteamId);
        st.Pos2 = pos;
        Reply(client, $"\x04[zone] Pos2: \x09{pos}");
        if (st.Pos1 is not null) Reply(client, $"\x08         Pos1: {st.Pos1}");
        return ECommandAction.Handled;
    }

    // ─── !addzone <type> [track] [seq] ─────────────────────────────────

    private ECommandAction CmdAddZone(IGameClient client, string arg)
    {
        if (!RequireAdmin(client, "!addzone")) return ECommandAction.Handled;
        var st = GetOrCreate((ulong)client.SteamId);
        if (st.Pos1 is null || st.Pos2 is null)
        {
            Reply(client, "\x07[zone] Set !pos1 and !pos2 first.");
            return ECommandAction.Handled;
        }

        var parts = arg.Split(' ', StringSplitOptions.RemoveEmptyEntries);
        if (parts.Length == 0 || !TryParseZoneType(parts[0], out var zoneType))
        {
            Reply(client, "\x07[zone] Usage: !addzone <start|end|stage|bonusstart|bonusend> [track] [seq]");
            return ECommandAction.Handled;
        }

        short track = 0, seq = 0;
        if (parts.Length >= 2) short.TryParse(parts[1], out track);
        if (parts.Length >= 3) short.TryParse(parts[2], out seq);

        var mins   = Vec3.Min(st.Pos1.Value, st.Pos2.Value);
        var maxs   = Vec3.Max(st.Pos1.Value, st.Pos2.Value);
        var center = Vec3.Center(mins, maxs);
        var mapId  = GetMapId(_currentMap);

        if (mapId <= 0)
        {
            Reply(client, $"\x07[zone] Map '{_currentMap}' not found in surf_maps. Complete a run first to register the map.");
            return ECommandAction.Handled;
        }

        try
        {
            using var conn = new MySqlConnection(_mysqlConnStr);
            conn.Open();
            using var cmd = new MySqlCommand(
                @"INSERT INTO surf_zones (MapId,Type,Track,Sequence,Mins,Maxs,Center,Angles,TeleportOrigin,TeleportAngles)
                  VALUES (@mapId,@type,@track,@seq,@mins,@maxs,@center,'0 0 0',@tp,'0 0 0')", conn);
            cmd.Parameters.AddWithValue("@mapId",  mapId);
            cmd.Parameters.AddWithValue("@type",   zoneType);
            cmd.Parameters.AddWithValue("@track",  track);
            cmd.Parameters.AddWithValue("@seq",    seq);
            cmd.Parameters.AddWithValue("@mins",   mins.ToString());
            cmd.Parameters.AddWithValue("@maxs",   maxs.ToString());
            cmd.Parameters.AddWithValue("@center", center.ToString());
            cmd.Parameters.AddWithValue("@tp",     new Vec3(center.X, center.Y, center.Z + 10).ToString());
            cmd.ExecuteNonQuery();
            var newId = cmd.LastInsertedId;

            _logger.LogInformation("ZoneEditor: INSERT id={Id} type={T} map={M}", newId, zoneType, _currentMap);
            RefreshZoneCache();
            if (_displayActive) _shared.GetModSharp().InvokeFrameAction(SpawnZoneDisplay);

            Reply(client, $"\x04[zone] Created \x09#{newId} \x04({TypeLabel(zoneType, track, seq)})");
            Reply(client, $"  Mins: {mins}  Maxs: {maxs}");
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "ZoneEditor: INSERT failed");
            Reply(client, "\x07[zone] DB error.");
        }
        return ECommandAction.Handled;
    }

    // ─── !editzone <id> ────────────────────────────────────────────────

    private ECommandAction CmdEditZone(IGameClient client, string arg)
    {
        if (!RequireAdmin(client, "!editzone")) return ECommandAction.Handled;
        if (!long.TryParse(arg, out var id))
        {
            Reply(client, "\x07[zone] Usage: !editzone <id>");
            return ECommandAction.Handled;
        }
        var z = _zoneCache.FirstOrDefault(x => x.Id == id) ?? FetchZone(id);
        if (z is null) { Reply(client, $"\x07[zone] Zone {id} not found."); return ECommandAction.Handled; }

        var st = GetOrCreate((ulong)client.SteamId);
        st.Pos1      = z.Mins;
        st.Pos2      = z.Maxs;
        st.EditingId = id;

        Reply(client, $"\x04[zone] Editing \x09#{id} \x04({z.TypeName})");
        Reply(client, $"  Mins loaded as Pos1: \x09{z.Mins}");
        Reply(client, $"  Maxs loaded as Pos2: \x09{z.Maxs}");
        Reply(client, "\x08  Walk to new corners → !pos1 / !pos2 → !savezone");
        return ECommandAction.Handled;
    }

    // ─── !savezone ─────────────────────────────────────────────────────

    private ECommandAction CmdSaveZone(IGameClient client)
    {
        if (!RequireAdmin(client, "!savezone")) return ECommandAction.Handled;
        var st = GetOrCreate((ulong)client.SteamId);

        if (st.EditingId is null)
        {
            Reply(client, "\x07[zone] No zone loaded. Use !editzone <id> first.");
            return ECommandAction.Handled;
        }
        if (st.Pos1 is null || st.Pos2 is null)
        {
            Reply(client, "\x07[zone] Set !pos1 and !pos2 first.");
            return ECommandAction.Handled;
        }

        var mins   = Vec3.Min(st.Pos1.Value, st.Pos2.Value);
        var maxs   = Vec3.Max(st.Pos1.Value, st.Pos2.Value);
        var center = Vec3.Center(mins, maxs);
        var zoneId = st.EditingId.Value;

        try
        {
            using var conn = new MySqlConnection(_mysqlConnStr);
            conn.Open();
            using var cmd = new MySqlCommand(
                "UPDATE surf_zones SET Mins=@mins,Maxs=@maxs,Center=@center,TeleportOrigin=@tp WHERE Id=@id", conn);
            cmd.Parameters.AddWithValue("@mins",   mins.ToString());
            cmd.Parameters.AddWithValue("@maxs",   maxs.ToString());
            cmd.Parameters.AddWithValue("@center", center.ToString());
            cmd.Parameters.AddWithValue("@tp",     new Vec3(center.X, center.Y, center.Z + 10).ToString());
            cmd.Parameters.AddWithValue("@id",     zoneId);
            if (cmd.ExecuteNonQuery() == 0)
            {
                Reply(client, $"\x07[zone] Zone {zoneId} not found in DB.");
                return ECommandAction.Handled;
            }

            _logger.LogInformation("ZoneEditor: UPDATE id={Id}", zoneId);
            st.EditingId = null;
            RefreshZoneCache();
            if (_displayActive) _shared.GetModSharp().InvokeFrameAction(SpawnZoneDisplay);

            Reply(client, $"\x04[zone] Saved \x09#{zoneId}\x04. Mins:{mins} Maxs:{maxs}");
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "ZoneEditor: UPDATE failed");
            Reply(client, "\x07[zone] DB error.");
        }
        return ECommandAction.Handled;
    }

    // ─── !delzone <id> [confirm] ───────────────────────────────────────

    private ECommandAction CmdDelZone(IGameClient client, string arg)
    {
        if (!RequireAdmin(client, "!delzone")) return ECommandAction.Handled;
        var parts = arg.Split(' ', 2, StringSplitOptions.RemoveEmptyEntries);
        if (parts.Length == 0 || !long.TryParse(parts[0], out var id))
        {
            Reply(client, "\x07[zone] Usage: !delzone <id>  then  !delzone <id> confirm");
            return ECommandAction.Handled;
        }

        if (parts.Length < 2 || !parts[1].Equals("confirm", StringComparison.OrdinalIgnoreCase))
        {
            var z = _zoneCache.FirstOrDefault(x => x.Id == id) ?? FetchZone(id);
            Reply(client, $"\x09[zone] About to delete \x01#{id} ({z?.TypeName ?? "?"}).");
            Reply(client, $"\x08  Type !delzone {id} confirm to proceed.");
            return ECommandAction.Handled;
        }

        try
        {
            using var conn = new MySqlConnection(_mysqlConnStr);
            conn.Open();
            using var cmd = new MySqlCommand("DELETE FROM surf_zones WHERE Id=@id", conn);
            cmd.Parameters.AddWithValue("@id", id);
            if (cmd.ExecuteNonQuery() == 0)
            {
                Reply(client, $"\x07[zone] Zone {id} not found.");
            }
            else
            {
                _logger.LogInformation("ZoneEditor: DELETE id={Id}", id);
                RefreshZoneCache();
                if (_displayActive) _shared.GetModSharp().InvokeFrameAction(SpawnZoneDisplay);
                Reply(client, $"\x04[zone] Deleted \x09#{id}\x04.");
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "ZoneEditor: DELETE failed");
            Reply(client, "\x07[zone] DB error.");
        }
        return ECommandAction.Handled;
    }

    // ─── !tpzone <id> ──────────────────────────────────────────────────

    private ECommandAction CmdTpZone(IGameClient client, string arg)
    {
        if (!RequireAdmin(client, "!tpzone")) return ECommandAction.Handled;
        if (!long.TryParse(arg, out var id))
        {
            Reply(client, "\x07[zone] Usage: !tpzone <id>");
            return ECommandAction.Handled;
        }
        var z = _zoneCache.FirstOrDefault(x => x.Id == id) ?? FetchZone(id);
        if (z is null) { Reply(client, $"\x07[zone] Zone {id} not found."); return ECommandAction.Handled; }

        var center = z.Center;
        _shared.GetModSharp().InvokeFrameAction(() =>
        {
            try
            {
                var pawn   = client.GetPlayerController()?.GetPlayerPawn();
                var entity = pawn as IBaseEntity;
                entity?.Teleport(new Vector(center.X, center.Y, center.Z + 10f), null, null);
            }
            catch (Exception ex) { _logger.LogWarning(ex, "ZoneEditor: teleport failed"); }
        });

        Reply(client, $"\x04[zone] Teleporting to \x09#{id} \x04({z.TypeName}) — {z.Center}");
        return ECommandAction.Handled;
    }

    // ─── Proximity tick ────────────────────────────────────────────────

    private readonly Dictionary<ulong, long?> _lastZoneId = [];

    private void ScheduleProximityTick()
        => _shared.GetModSharp().PushTimer(ProximityTick, 3.0f, GameTimerFlags.None);

    private void ProximityTick()
    {
        ScheduleProximityTick();
        if (_liveAlert.Count == 0 || _zoneCache.Count == 0) return;
        try
        {
            foreach (var steamIdU in _liveAlert)
            {
                IGameClient? target = null;
                for (byte slot = 0; slot < 64; slot++)
                {
                    var c = _clientManager.GetGameClient(new PlayerSlot(slot));
                    if (c is null || c.IsFakeClient) continue;
                    if ((ulong)c.SteamId == steamIdU) { target = c; break; }
                }
                if (target is null) continue;

                var pos = GetPlayerPosition(target);
                if (pos is null) continue;

                long? cur = null;
                foreach (var z in _zoneCache)
                    if (pos.Value.Inside(z.Mins, z.Maxs)) { cur = z.Id; break; }

                _lastZoneId.TryGetValue(steamIdU, out var prev);
                if (cur == prev) continue;
                _lastZoneId[steamIdU] = cur;

                if (cur is not null)
                {
                    var z = _zoneCache.First(x => x.Id == cur);
                    Reply(target, $"\x04[zone] ENTER \x09#{z.Id} \x04({z.TypeName})  {z.Mins} → {z.Maxs}");
                }
                else
                {
                    Reply(target, "\x08[zone] LEFT zone.");
                }
            }
        }
        catch (Exception ex) { _logger.LogError(ex, "ZoneEditor: ProximityTick failed"); }
    }

    // ─── DB ────────────────────────────────────────────────────────────

    private void RefreshZoneCache()
    {
        if (string.IsNullOrEmpty(_mysqlConnStr) || string.IsNullOrEmpty(_currentMap))
        {
            _zoneCache = [];
            return;
        }
        try
        {
            _zoneCache = QueryZones(_currentMap);
            _logger.LogInformation("ZoneEditor: {N} zones for {M}", _zoneCache.Count, _currentMap);
        }
        catch (Exception ex) { _logger.LogError(ex, "ZoneEditor: RefreshZoneCache failed"); }
    }

    private List<ZoneRow> QueryZones(string mapFile)
    {
        using var conn = new MySqlConnection(_mysqlConnStr);
        conn.Open();
        using var cmd = new MySqlCommand(
            @"SELECT z.Id,z.MapId,z.Type,z.Track,z.Sequence,z.Mins,z.Maxs,z.Center
              FROM surf_zones z JOIN surf_maps m ON m.MapId=z.MapId
              WHERE m.File=@f ORDER BY z.Type,z.Track,z.Sequence", conn);
        cmd.Parameters.AddWithValue("@f", mapFile);
        using var r = cmd.ExecuteReader();
        var result = new List<ZoneRow>();
        while (r.Read())
        {
            var mins   = Vec3.TryParse(r.GetString(5)) ?? default;
            var maxs   = Vec3.TryParse(r.GetString(6)) ?? default;
            var center = Vec3.TryParse(r.GetString(7)) ?? Vec3.Center(mins, maxs);
            result.Add(new ZoneRow
            {
                Id = r.GetInt64(0), MapId = r.GetInt64(1), Type = r.GetInt32(2),
                Track = r.GetInt16(3), Sequence = r.GetInt16(4),
                Mins = mins, Maxs = maxs, Center = center,
            });
        }
        return result;
    }

    private ZoneRow? FetchZone(long id)
    {
        if (string.IsNullOrEmpty(_mysqlConnStr)) return null;
        try
        {
            using var conn = new MySqlConnection(_mysqlConnStr);
            conn.Open();
            using var cmd = new MySqlCommand(
                "SELECT Id,MapId,Type,Track,Sequence,Mins,Maxs,Center FROM surf_zones WHERE Id=@id", conn);
            cmd.Parameters.AddWithValue("@id", id);
            using var r = cmd.ExecuteReader();
            if (!r.Read()) return null;
            var mins   = Vec3.TryParse(r.GetString(5)) ?? default;
            var maxs   = Vec3.TryParse(r.GetString(6)) ?? default;
            var center = Vec3.TryParse(r.GetString(7)) ?? Vec3.Center(mins, maxs);
            return new ZoneRow
            {
                Id = r.GetInt64(0), MapId = r.GetInt64(1), Type = r.GetInt32(2),
                Track = r.GetInt16(3), Sequence = r.GetInt16(4),
                Mins = mins, Maxs = maxs, Center = center,
            };
        }
        catch (Exception ex) { _logger.LogError(ex, "ZoneEditor: FetchZone {Id} failed", id); return null; }
    }

    private long GetMapId(string mapFile)
    {
        if (string.IsNullOrEmpty(_mysqlConnStr) || string.IsNullOrEmpty(mapFile)) return -1;
        try
        {
            using var conn = new MySqlConnection(_mysqlConnStr);
            conn.Open();
            using var cmd = new MySqlCommand("SELECT MapId FROM surf_maps WHERE File=@f LIMIT 1", conn);
            cmd.Parameters.AddWithValue("@f", mapFile);
            var result = cmd.ExecuteScalar();
            return result is not null ? Convert.ToInt64(result) : -1;
        }
        catch (Exception ex) { _logger.LogError(ex, "ZoneEditor: GetMapId failed"); return -1; }
    }

    // ─── Helpers ───────────────────────────────────────────────────────

    private Vec3? GetPlayerPosition(IGameClient client)
    {
        try
        {
            var pawn   = client.GetPlayerController()?.GetPlayerPawn();
            var entity = pawn as IBaseEntity;
            if (entity is null) return null;
            var origin = entity.GetAbsOrigin();
            return new Vec3(origin.X, origin.Y, origin.Z);
        }
        catch (Exception ex) { _logger.LogWarning(ex, "ZoneEditor: GetPlayerPosition failed"); return null; }
    }

    private void Reply(IGameClient client, string msg)
    {
        try { client.GetPlayerController()?.GetPlayerPawn()?.Print(HudPrintChannel.Chat, msg); }
        catch { }
    }

    private bool RequireAdmin(IGameClient client, string action)
    {
        if (_envAdminIds.Contains((ulong)client.SteamId)) return true;
#pragma warning disable CS0618
        var admin = _clientManager.FindAdmin(client.SteamId);
#pragma warning restore CS0618
        if (admin is not null && (admin.HasPermission("admin:zone") || admin.HasPermission("*")))
            return true;
        Reply(client, $"\x07[zone] {action}: admin required.");
        return false;
    }

    private static bool TryParseZoneType(string s, out int zoneType)
    {
        zoneType = s.ToLowerInvariant() switch
        {
            "start" or "s" or "0"                   => 0,
            "end" or "finish" or "e" or "f" or "1"  => 1,
            "stage" or "st" or "2"                   => 2,
            "bonusstart" or "bs" or "3"              => 3,
            "bonusend" or "be" or "4"                => 4,
            _                                        => -1,
        };
        return zoneType >= 0;
    }

    private static string TypeLabel(int type, short track, short seq) => type switch
    {
        0 => "Start", 1 => "End", 2 => $"Stage {seq}",
        3 => $"BonusStart T{track}", 4 => $"BonusEnd T{track}", _ => $"Type{type}",
    };

    private AdminState GetOrCreate(ulong id)
    {
        if (!_state.TryGetValue(id, out var st)) { st = new AdminState(); _state[id] = st; }
        return st;
    }
}
