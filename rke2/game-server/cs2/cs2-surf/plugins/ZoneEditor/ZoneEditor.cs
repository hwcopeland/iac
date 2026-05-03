/*
 * ZoneEditor — live zone inspection and editing for Source2Surf.
 *
 * Admin commands (require "admin:zone" or "*" permission, or MAP_ADMIN_STEAMIDS):
 *   !zones [page]            - list zones for current map (8 per page)
 *   !zoneinfo <id>           - dump full zone details
 *   !pos1                    - mark corner-1 at your current position
 *   !pos2                    - mark corner-2 at your current position
 *   !addzone <type> [track] [seq] - create zone from pos1+pos2
 *   !editzone <id>           - load existing zone into pos1/pos2
 *   !savezone                - write pos1/pos2 back to the zone loaded with !editzone
 *   !delzone <id>            - delete a zone by ID (asks confirmation with !delzone <id> confirm)
 *   !tpzone <id>             - teleport to zone center
 *   !showzones               - toggle live proximity alerts for your session
 *
 * Zone type keywords for !addzone:
 *   start / s         → 0  (map start, track 0)
 *   end / finish / e  → 1  (map end)
 *   stage / st        → 2  (stage start — !addzone stage 0 <seq>)
 *   bonusstart / bs   → 3  (bonus start — !addzone bonusstart <track>)
 *   bonusend / be     → 4  (bonus end   — !addzone bonusend <track>)
 *
 * Example workflow for surf_oasis stage 1:
 *   Walk to start trigger corner-A → !pos1
 *   Walk to start trigger corner-B → !pos2
 *   !addzone stage 0 1
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

    // Per-admin edit state keyed by SteamID (as ulong).
    private readonly Dictionary<ulong, AdminState> _state = [];

    // Admins who want live proximity zone alerts.
    private readonly HashSet<ulong> _liveAlert = [];

    // Cached zone list for current map. Refreshed on map load and after writes.
    private List<ZoneRow> _zoneCache = [];

    // Env-configured fallback admin IDs (same env var as SurfMapCommand).
    private readonly HashSet<ulong> _envAdminIds = [];

    private sealed class AdminState
    {
        public Vec3? Pos1;
        public Vec3? Pos2;
        public long? EditingId;
        public long? PendingDeleteId;
    }

    private readonly record struct Vec3(float X, float Y, float Z)
    {
        public override string ToString() => $"{X:F2} {Y:F2} {Z:F2}";

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
        {
            if (ulong.TryParse(part, out var id)) _envAdminIds.Add(id);
        }

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
            _logger.LogWarning("ZoneEditor: MYSQL_HOST/MYSQL_USER not set — zone DB commands disabled");
        }

        _clientManager.InstallClientListener(this);
        _shared.GetModSharp().InstallGameListener(this);

        // Schedule periodic proximity check for admins in live-alert mode.
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
        _zoneCache.Clear();
        _currentMap = "";

        // Grab map name from env var set in K8s as a bootstrap value.
        // Source2Surf's Timer also sets up zones keyed by map file name.
        var envMap = Environment.GetEnvironmentVariable("MAP") ?? "";
        if (!string.IsNullOrEmpty(envMap))
        {
            // Strip workshop prefix (e.g. "3070923090" → needs lookup; plain names kept as-is).
            _currentMap = envMap;
        }

        RefreshZoneCache();
        _logger.LogInformation("ZoneEditor: OnServerActivate map={Map} zones={Z}", _currentMap, _zoneCache.Count);
    }

    public void OnClientPutInServer(IGameClient client)  { }
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

        var body = raw.Substring(1);
        if (body.Length == 0) return ECommandAction.Skipped;

        var spaceIdx = body.IndexOf(' ');
        var cmd = (spaceIdx < 0 ? body : body[..spaceIdx]).ToLowerInvariant();
        var arg = (spaceIdx < 0 ? "" : body[(spaceIdx + 1)..]).Trim();

        return cmd switch
        {
            "zones"                          => CmdZones(client, arg),
            "zoneinfo"                       => CmdZoneInfo(client, arg),
            "pos1"                           => CmdPos1(client),
            "pos2"                           => CmdPos2(client),
            "addzone"                        => CmdAddZone(client, arg),
            "editzone"                       => CmdEditZone(client, arg),
            "savezone"                       => CmdSaveZone(client),
            "delzone"                        => CmdDelZone(client, arg),
            "tpzone"                         => CmdTpZone(client, arg),
            "showzones"                      => CmdShowZones(client),
            "setmap"                         => CmdSetMap(client, arg),
            _                                => ECommandAction.Skipped,
        };
    }

    // ─── !zones [page] ─────────────────────────────────────────────────

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
        var zones = GetZonesForCurrentMap();
        var total = zones.Count;
        var pages = Math.Max(1, (total + perPage - 1) / perPage);
        if (page > pages) page = pages;

        var slice = zones.Skip((page - 1) * perPage).Take(perPage).ToList();

        Reply(client, $"\x04===== Zones: {_currentMap} ({total}) pg {page}/{pages} =====");
        if (slice.Count == 0)
        {
            Reply(client, "\x08  (none)");
        }
        else
        {
            foreach (var z in slice)
                Reply(client, $"  \x09ID {z.Id} \x08| \x0B{z.TypeName} \x08| Mins:{z.Mins} Maxs:{z.Maxs}");
        }
        Reply(client, "\x08  !zones [page]  !zoneinfo <id>  !tpzone <id>");
        return ECommandAction.Handled;
    }

    // ─── !zoneinfo <id> ────────────────────────────────────────────────

    private ECommandAction CmdZoneInfo(IGameClient client, string arg)
    {
        if (!long.TryParse(arg, out var id))
        {
            Reply(client, "\x07[zone] Usage: !zoneinfo <id>");
            return ECommandAction.Handled;
        }

        var z = _zoneCache.FirstOrDefault(x => x.Id == id)
             ?? FetchZone(id);

        if (z is null)
        {
            Reply(client, $"\x07[zone] Zone {id} not found.");
            return ECommandAction.Handled;
        }

        Reply(client, $"\x04===== Zone {z.Id} =====");
        Reply(client, $"  \x09Type: \x01{z.TypeName}  Track={z.Track}  Seq={z.Sequence}");
        Reply(client, $"  \x09Mins: \x01{z.Mins}");
        Reply(client, $"  \x09Maxs: \x01{z.Maxs}");
        Reply(client, $"  \x09Center: \x01{z.Center}");
        return ECommandAction.Handled;
    }

    // ─── !pos1 / !pos2 ─────────────────────────────────────────────────

    private ECommandAction CmdPos1(IGameClient client)
    {
        if (!RequireAdmin(client, "!pos1")) return ECommandAction.Handled;
        var pos = GetPlayerPosition(client);
        if (pos is null) { Reply(client, "\x07[zone] Cannot read your position."); return ECommandAction.Handled; }

        var st = GetOrCreate((ulong)client.SteamId);
        st.Pos1 = pos;
        Reply(client, $"\x04[zone] Pos1 set: \x09{pos}");
        if (st.Pos2 is not null)
            Reply(client, $"\x08[zone] Pos2: {st.Pos2}  — ready for !addzone or !savezone");
        return ECommandAction.Handled;
    }

    private ECommandAction CmdPos2(IGameClient client)
    {
        if (!RequireAdmin(client, "!pos2")) return ECommandAction.Handled;
        var pos = GetPlayerPosition(client);
        if (pos is null) { Reply(client, "\x07[zone] Cannot read your position."); return ECommandAction.Handled; }

        var st = GetOrCreate((ulong)client.SteamId);
        st.Pos2 = pos;
        Reply(client, $"\x04[zone] Pos2 set: \x09{pos}");
        if (st.Pos1 is not null)
            Reply(client, $"\x08[zone] Pos1: {st.Pos1}  — ready for !addzone or !savezone");
        return ECommandAction.Handled;
    }

    // ─── !addzone <type> [track=0] [seq=0] ─────────────────────────────

    private ECommandAction CmdAddZone(IGameClient client, string arg)
    {
        if (!RequireAdmin(client, "!addzone")) return ECommandAction.Handled;

        var st = GetOrCreate((ulong)client.SteamId);
        if (st.Pos1 is null || st.Pos2 is null)
        {
            Reply(client, "\x07[zone] Set !pos1 and !pos2 first.");
            return ECommandAction.Handled;
        }

        // Parse: <type> [track] [seq]
        var parts = arg.Split(' ', StringSplitOptions.RemoveEmptyEntries);
        if (parts.Length == 0)
        {
            Reply(client, "\x07[zone] Usage: !addzone <start|end|stage|bonusstart|bonusend> [track] [seq]");
            return ECommandAction.Handled;
        }

        if (!TryParseZoneType(parts[0], out var zoneType))
        {
            Reply(client, $"\x07[zone] Unknown type '{parts[0]}'. Use: start end stage bonusstart bonusend");
            return ECommandAction.Handled;
        }

        short track = 0, seq = 0;
        if (parts.Length >= 2) short.TryParse(parts[1], out track);
        if (parts.Length >= 3) short.TryParse(parts[2], out seq);

        var mins   = Vec3.Min(st.Pos1.Value, st.Pos2.Value);
        var maxs   = Vec3.Max(st.Pos1.Value, st.Pos2.Value);
        var center = Vec3.Center(mins, maxs);

        var mapId = GetOrCreateMapId(_currentMap);
        if (mapId <= 0)
        {
            Reply(client, $"\x07[zone] Cannot resolve map '{_currentMap}' in surf_maps. Use !setmap <name> to fix.");
            return ECommandAction.Handled;
        }

        try
        {
            using var conn = new MySqlConnection(_mysqlConnStr);
            conn.Open();
            using var cmd = new MySqlCommand(
                @"INSERT INTO surf_zones (MapId, Type, Track, Sequence, Mins, Maxs, Center, Angles, TeleportOrigin, TeleportAngles)
                  VALUES (@mapId, @type, @track, @seq, @mins, @maxs, @center, '0 0 0', @tpOrigin, '0 0 0')",
                conn);
            cmd.Parameters.AddWithValue("@mapId",    mapId);
            cmd.Parameters.AddWithValue("@type",     zoneType);
            cmd.Parameters.AddWithValue("@track",    track);
            cmd.Parameters.AddWithValue("@seq",      seq);
            cmd.Parameters.AddWithValue("@mins",     mins.ToString());
            cmd.Parameters.AddWithValue("@maxs",     maxs.ToString());
            cmd.Parameters.AddWithValue("@center",   center.ToString());
            // Teleport origin = center raised by 10 units so spawn is inside zone not in floor.
            cmd.Parameters.AddWithValue("@tpOrigin", new Vec3(center.X, center.Y, center.Z + 10).ToString());
            cmd.ExecuteNonQuery();
            var newId = cmd.LastInsertedId;

            _logger.LogInformation("ZoneEditor: INSERT zone id={Id} type={T} map={M}", newId, zoneType, _currentMap);
            RefreshZoneCache();
            Reply(client, $"\x04[zone] Created zone ID \x09{newId} \x04({TypeName(zoneType, track, seq)}) — Mins:{mins} Maxs:{maxs}");
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "ZoneEditor: INSERT failed");
            Reply(client, "\x07[zone] DB error creating zone.");
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

        var z = FetchZone(id);
        if (z is null)
        {
            Reply(client, $"\x07[zone] Zone {id} not found.");
            return ECommandAction.Handled;
        }

        var st = GetOrCreate((ulong)client.SteamId);
        st.Pos1       = z.Mins;
        st.Pos2       = z.Maxs;
        st.EditingId  = id;

        Reply(client, $"\x04[zone] Editing zone \x09{id} \x04({z.TypeName})");
        Reply(client, $"  Pos1 (Mins): \x09{z.Mins}");
        Reply(client, $"  Pos2 (Maxs): \x09{z.Maxs}");
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
                @"UPDATE surf_zones SET Mins=@mins, Maxs=@maxs, Center=@center, TeleportOrigin=@tp
                  WHERE Id=@id",
                conn);
            cmd.Parameters.AddWithValue("@mins",   mins.ToString());
            cmd.Parameters.AddWithValue("@maxs",   maxs.ToString());
            cmd.Parameters.AddWithValue("@center", center.ToString());
            cmd.Parameters.AddWithValue("@tp",     new Vec3(center.X, center.Y, center.Z + 10).ToString());
            cmd.Parameters.AddWithValue("@id",     zoneId);
            var rows = cmd.ExecuteNonQuery();

            if (rows == 0)
            {
                Reply(client, $"\x07[zone] Zone {zoneId} not found in DB.");
                return ECommandAction.Handled;
            }

            _logger.LogInformation("ZoneEditor: UPDATE zone id={Id} mins={M} maxs={X}", zoneId, mins, maxs);
            st.EditingId = null;
            RefreshZoneCache();
            Reply(client, $"\x04[zone] Saved zone \x09{zoneId}\x04. Mins:{mins} Maxs:{maxs}");
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "ZoneEditor: UPDATE failed");
            Reply(client, "\x07[zone] DB error saving zone.");
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
            Reply(client, "\x07[zone] Usage: !delzone <id>  (then !delzone <id> confirm)");
            return ECommandAction.Handled;
        }

        var st = GetOrCreate((ulong)client.SteamId);

        // Two-step confirmation.
        if (parts.Length < 2 || !parts[1].Equals("confirm", StringComparison.OrdinalIgnoreCase))
        {
            var z = _zoneCache.FirstOrDefault(x => x.Id == id) ?? FetchZone(id);
            st.PendingDeleteId = id;
            Reply(client, $"\x09[zone] About to delete zone {id} ({z?.TypeName ?? "?"}). Type !delzone {id} confirm to proceed.");
            return ECommandAction.Handled;
        }

        // Confirmed.
        st.PendingDeleteId = null;
        try
        {
            using var conn = new MySqlConnection(_mysqlConnStr);
            conn.Open();
            using var cmd = new MySqlCommand("DELETE FROM surf_zones WHERE Id=@id", conn);
            cmd.Parameters.AddWithValue("@id", id);
            var rows = cmd.ExecuteNonQuery();

            if (rows == 0)
            {
                Reply(client, $"\x07[zone] Zone {id} not found.");
            }
            else
            {
                _logger.LogInformation("ZoneEditor: DELETE zone id={Id}", id);
                RefreshZoneCache();
                Reply(client, $"\x04[zone] Deleted zone \x09{id}\x04.");
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "ZoneEditor: DELETE failed");
            Reply(client, "\x07[zone] DB error deleting zone.");
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
        if (z is null)
        {
            Reply(client, $"\x07[zone] Zone {id} not found.");
            return ECommandAction.Handled;
        }

        try
        {
            var center = z.Center;
            // Teleport is on IBaseEntity — cast from IPlayerPawn before calling.
            _shared.GetModSharp().InvokeFrameAction(() =>
            {
                try
                {
                    var ctrl = client.GetPlayerController();
                    var pawn = ctrl?.GetPlayerPawn();
                    if (pawn is null) return;
                    var entity = pawn as IBaseEntity;
                    entity?.Teleport(new Sharp.Shared.Types.Vector(center.X, center.Y, center.Z + 10), null, null);
                }
                catch (Exception ex)
                {
                    _logger.LogWarning(ex, "ZoneEditor: teleport frame action failed");
                }
            });
            Reply(client, $"\x04[zone] Teleporting to zone \x09{id} \x04({z.TypeName}) center {z.Center}");
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "ZoneEditor: !tpzone failed");
            Reply(client, "\x07[zone] Teleport failed.");
        }

        return ECommandAction.Handled;
    }

    // ─── !showzones ────────────────────────────────────────────────────

    private ECommandAction CmdShowZones(IGameClient client)
    {
        if (!RequireAdmin(client, "!showzones")) return ECommandAction.Handled;

        var id = (ulong)client.SteamId;
        if (_liveAlert.Contains(id))
        {
            _liveAlert.Remove(id);
            Reply(client, "\x08[zone] Live zone alerts OFF.");
        }
        else
        {
            _liveAlert.Add(id);
            Reply(client, "\x04[zone] Live zone alerts ON — alerts fire when you enter/leave a zone boundary.");
        }
        return ECommandAction.Handled;
    }

    // ─── !setmap <mapname> ─────────────────────────────────────────────
    // Override map context for zone queries (useful when MAP env var doesn't match DB file name).

    private ECommandAction CmdSetMap(IGameClient client, string arg)
    {
        if (!RequireAdmin(client, "!setmap")) return ECommandAction.Handled;

        if (string.IsNullOrEmpty(arg))
        {
            Reply(client, $"\x08[zone] Current map context: \x09{_currentMap}");
            Reply(client, "\x08  !setmap <mapname> to override (e.g. !setmap surf_oasis)");
            return ECommandAction.Handled;
        }

        _currentMap = arg.Trim().ToLowerInvariant();
        RefreshZoneCache();
        Reply(client, $"\x04[zone] Map context set to \x09{_currentMap} \x04({_zoneCache.Count} zones loaded)");
        return ECommandAction.Handled;
    }

    // ─── Proximity tick ────────────────────────────────────────────────

    private readonly Dictionary<ulong, long?> _lastZoneId = [];

    private void ScheduleProximityTick()
    {
        _shared.GetModSharp().PushTimer(ProximityTick, 3.0f, GameTimerFlags.None);
    }

    private void ProximityTick()
    {
        ScheduleProximityTick();
        if (_liveAlert.Count == 0 || _zoneCache.Count == 0) return;

        try
        {
            foreach (var steamIdU in _liveAlert)
            {
                // Find the client by iterating slots.
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

                long? currentZone = null;
                foreach (var z in _zoneCache)
                {
                    if (pos.Value.Inside(z.Mins, z.Maxs))
                    {
                        currentZone = z.Id;
                        break;
                    }
                }

                _lastZoneId.TryGetValue(steamIdU, out var prevZone);
                if (currentZone != prevZone)
                {
                    _lastZoneId[steamIdU] = currentZone;
                    if (currentZone is not null)
                    {
                        var z = _zoneCache.First(x => x.Id == currentZone);
                        Reply(target, $"\x04[zone] ENTER zone \x09{z.Id} \x08({z.TypeName})  Mins:{z.Mins}  Maxs:{z.Maxs}");
                    }
                    else
                    {
                        Reply(target, "\x08[zone] LEFT zone.");
                    }
                }
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "ZoneEditor: ProximityTick failed");
        }
    }

    // ─── DB helpers ────────────────────────────────────────────────────

    private void RefreshZoneCache()
    {
        if (string.IsNullOrEmpty(_mysqlConnStr) || string.IsNullOrEmpty(_currentMap))
        {
            _zoneCache.Clear();
            return;
        }

        try
        {
            _zoneCache = GetZonesForCurrentMap();
            _logger.LogInformation("ZoneEditor: zone cache refreshed ({N} zones for {M})", _zoneCache.Count, _currentMap);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "ZoneEditor: RefreshZoneCache failed");
        }
    }

    private List<ZoneRow> GetZonesForCurrentMap()
    {
        if (string.IsNullOrEmpty(_mysqlConnStr)) return [];

        try
        {
            using var conn = new MySqlConnection(_mysqlConnStr);
            conn.Open();
            using var cmd = new MySqlCommand(
                @"SELECT z.Id, z.MapId, z.Type, z.Track, z.Sequence, z.Mins, z.Maxs, z.Center
                  FROM surf_zones z
                  JOIN surf_maps m ON m.MapId = z.MapId
                  WHERE m.File = @mapFile
                  ORDER BY z.Type, z.Track, z.Sequence",
                conn);
            cmd.Parameters.AddWithValue("@mapFile", _currentMap);
            using var reader = cmd.ExecuteReader();

            var result = new List<ZoneRow>();
            while (reader.Read())
            {
                var mins   = Vec3.TryParse(reader.GetString(5)) ?? default;
                var maxs   = Vec3.TryParse(reader.GetString(6)) ?? default;
                var center = Vec3.TryParse(reader.GetString(7)) ?? Vec3.Center(mins, maxs);
                result.Add(new ZoneRow
                {
                    Id       = reader.GetInt64(0),
                    MapId    = reader.GetInt64(1),
                    Type     = reader.GetInt32(2),
                    Track    = reader.GetInt16(3),
                    Sequence = reader.GetInt16(4),
                    Mins     = mins,
                    Maxs     = maxs,
                    Center   = center,
                });
            }
            return result;
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "ZoneEditor: GetZonesForCurrentMap failed");
            return [];
        }
    }

    private ZoneRow? FetchZone(long id)
    {
        if (string.IsNullOrEmpty(_mysqlConnStr)) return null;
        try
        {
            using var conn = new MySqlConnection(_mysqlConnStr);
            conn.Open();
            using var cmd = new MySqlCommand(
                "SELECT Id, MapId, Type, Track, Sequence, Mins, Maxs, Center FROM surf_zones WHERE Id=@id",
                conn);
            cmd.Parameters.AddWithValue("@id", id);
            using var r = cmd.ExecuteReader();
            if (!r.Read()) return null;
            var mins   = Vec3.TryParse(r.GetString(5)) ?? default;
            var maxs   = Vec3.TryParse(r.GetString(6)) ?? default;
            var center = Vec3.TryParse(r.GetString(7)) ?? Vec3.Center(mins, maxs);
            return new ZoneRow
            {
                Id       = r.GetInt64(0),
                MapId    = r.GetInt64(1),
                Type     = r.GetInt32(2),
                Track    = r.GetInt16(3),
                Sequence = r.GetInt16(4),
                Mins     = mins,
                Maxs     = maxs,
                Center   = center,
            };
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "ZoneEditor: FetchZone {Id} failed", id);
            return null;
        }
    }

    private long GetOrCreateMapId(string mapFile)
    {
        if (string.IsNullOrEmpty(_mysqlConnStr) || string.IsNullOrEmpty(mapFile)) return -1;
        try
        {
            using var conn = new MySqlConnection(_mysqlConnStr);
            conn.Open();
            using var cmd = new MySqlCommand(
                "SELECT MapId FROM surf_maps WHERE File = @f LIMIT 1",
                conn);
            cmd.Parameters.AddWithValue("@f", mapFile);
            var result = cmd.ExecuteScalar();
            if (result is not null) return Convert.ToInt64(result);
            return -1;
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "ZoneEditor: GetOrCreateMapId failed");
            return -1;
        }
    }

    // ─── Helpers ───────────────────────────────────────────────────────

    private Vec3? GetPlayerPosition(IGameClient client)
    {
        try
        {
            var ctrl = client.GetPlayerController();
            var pawn = ctrl?.GetPlayerPawn();
            if (pawn is null) return null;
            // AbsOrigin is defined on IBaseEntity; cast through since IPlayerPawn doesn't
            // expose it directly in the C# interface even though it implements it at runtime.
            var entity = pawn as IBaseEntity;
            if (entity is null) return null;
            var origin = entity.AbsOrigin;
            return new Vec3(origin.X, origin.Y, origin.Z);
        }
        catch (Exception ex)
        {
            _logger.LogWarning(ex, "ZoneEditor: GetPlayerPosition failed");
            return null;
        }
    }

    private void Reply(IGameClient client, string msg)
    {
        try
        {
            var pawn = client.GetPlayerController()?.GetPlayerPawn();
            pawn?.Print(HudPrintChannel.Chat, msg);
        }
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
            "start" or "s" or "0"                       => 0,
            "end" or "finish" or "e" or "f" or "1"     => 1,
            "stage" or "st" or "2"                      => 2,
            "bonusstart" or "bs" or "3"                 => 3,
            "bonusend" or "be" or "4"                   => 4,
            _                                            => -1,
        };
        return zoneType >= 0;
    }

    private static string TypeName(int type, short track, short seq) => type switch
    {
        0 => "Start",
        1 => "End",
        2 => $"Stage {seq}",
        3 => $"BonusStart T{track}",
        4 => $"BonusEnd T{track}",
        _ => $"Type{type}",
    };

    private AdminState GetOrCreate(ulong id)
    {
        if (!_state.TryGetValue(id, out var st))
        {
            st = new AdminState();
            _state[id] = st;
        }
        return st;
    }
}
