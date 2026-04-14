/*
 * SurfMapCommand — native ModSharp module providing all server map commands.
 *
 *   !map <id|name>   admin:map — immediate change
 *   !rtv             any player — majority vote rotates immediately
 *   !nominate <id>   any player — stores workshop id as the next vote pick
 *   !extend          any player — majority vote pushes rotation deadline
 *                    by MAP_EXTEND_MINUTES; capped at MAP_MAX_EXTENDS per map
 *
 * Map rotation runs inside the plugin on a self-rescheduling ModSharp
 * timer, so !extend and !rtv can mutate the single _nextRotationAt
 * DateTime rather than coordinating with a shell-level rotation loop.
 *
 * Config via env vars (read once at PostInit):
 *   MAP_ROTATION_MINUTES   default 30
 *   MAP_EXTEND_MINUTES     default 15
 *   MAP_MAX_EXTENDS        default 3
 *
 * Map list: configs/maprotation.txt on the PVC, one workshop ID per line,
 * comments with '#'. Rotation cycles through sequentially.
 */

using System;
using System.Collections.Generic;
using System.IO;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging;
using Sharp.Shared;
using Sharp.Shared.Enums;
using Sharp.Shared.Listeners;
using Sharp.Shared.Managers;
using Sharp.Shared.Objects;
using Sharp.Shared.Types;
using Sharp.Shared.Units;

namespace Cs2Surf.MapCommand;

public sealed class SurfMapCommand : IModSharpModule, IClientListener, IGameListener
{
    public string DisplayName   => "Cs2Surf.MapCommand";
    public string DisplayAuthor => "hwcopeland";

    private readonly ISharedSystem           _shared;
    private readonly ILogger<SurfMapCommand> _logger;
    private readonly IClientManager          _clientManager;
    private readonly string                  _sharpPath;

    // --- RTV / extend vote state ---
    private readonly HashSet<SteamID> _rtvVoters    = [];
    private readonly HashSet<SteamID> _extendVoters = [];
    private          string?         _lastNomination;
    private          int             _extendsUsed;

    // --- Rotation state ---
    private DateTime _nextRotationAt;
    private int      _rotationIndex;

    // Config
    private int _rotationMinutes = 30;
    private int _extendMinutes   = 15;
    private int _maxExtends      = 3;

    // Env-configured allowlist of SteamIDs (SteamID64) that can use !map,
    // bypassing ModSharp's AdminManager. Filled from MAP_ADMIN_STEAMIDS.
    // Belt-and-suspenders fallback if FindAdmin / HasPermission doesn't
    // resolve the admins.jsonc @root → "*" chain as expected.
    private readonly HashSet<ulong> _envAdminIds = [];

    public SurfMapCommand(ISharedSystem sharedSystem,
                          string        dllPath,
                          string        sharpPath,
                          Version       version,
                          IConfiguration coreConfiguration,
                          bool          hotReload)
    {
        _shared        = sharedSystem;
        _logger        = sharedSystem.GetLoggerFactory().CreateLogger<SurfMapCommand>();
        _clientManager = sharedSystem.GetClientManager();
        _sharpPath     = sharpPath;
    }

    public bool Init() => true;

    public void PostInit()
    {
        _rotationMinutes = ReadIntEnv("MAP_ROTATION_MINUTES", 30);
        _extendMinutes   = ReadIntEnv("MAP_EXTEND_MINUTES",   15);
        _maxExtends      = ReadIntEnv("MAP_MAX_EXTENDS",       3);

        var envAdmins = Environment.GetEnvironmentVariable("MAP_ADMIN_STEAMIDS")
                        ?? string.Empty;
        foreach (var part in envAdmins.Split(',', StringSplitOptions.RemoveEmptyEntries
                                                   | StringSplitOptions.TrimEntries))
        {
            if (ulong.TryParse(part, out var id))
            {
                _envAdminIds.Add(id);
            }
        }

        _nextRotationAt = DateTime.UtcNow.AddMinutes(_rotationMinutes);

        _clientManager.InstallClientListener(this);
        _shared.GetModSharp().InstallGameListener(this);
        ScheduleTick();

        _logger.LogInformation(
            "SurfMapCommand loaded — rotation={Rotation}m extend={Extend}m maxExtends={MaxExt}; envAdmins={AdminCount}; next rotation at {NextAt:u}",
            _rotationMinutes, _extendMinutes, _maxExtends, _envAdminIds.Count, _nextRotationAt);
    }

    public void Shutdown()
    {
        _clientManager.RemoveClientListener(this);
        _shared.GetModSharp().RemoveGameListener(this);
    }

    int IClientListener.ListenerVersion  => IClientListener.ApiVersion;
    int IClientListener.ListenerPriority => 0;

    int IGameListener.ListenerVersion  => IGameListener.ApiVersion;
    int IGameListener.ListenerPriority => 0;

    // Re-arm the rotation tick on every map activation. PushTimer state is
    // cleared across map changes, so without this the timer dies after the
    // first !map / !rtv / scheduled rotation.
    void IGameListener.OnServerActivate()
    {
        _logger.LogInformation("OnServerActivate — re-arming rotation tick");
        ScheduleTick();
        // Reset per-map vote state so a new map starts with clean slate.
        _rtvVoters.Clear();
        _extendVoters.Clear();
        _extendsUsed    = 0;
        _lastNomination = null;
    }

    // ------------------------------------------------------------------
    // Chat dispatch
    // ------------------------------------------------------------------

    public ECommandAction OnClientSayCommand(IGameClient client,
                                             bool        teamOnly,
                                             bool        isCommand,
                                             string      commandName,
                                             string      message)
    {
        // Brute-force diagnostic — remove once everything's confirmed working.
        _logger.LogInformation(
            "SAY from {SteamId} team={Team} isCmd={IsCmd} cmd={Cmd} msg={Msg}",
            client.SteamId, teamOnly, isCommand, commandName, message);

        if (!isCommand)
        {
            return ECommandAction.Skipped;
        }

        // ModSharp passes commandName="say" and the entire chat line in
        // message (e.g. "!map surf_kitsune"). The real command + args have
        // to be parsed out of message, not commandName.
        var body = (message ?? string.Empty).TrimStart();
        if (body.Length == 0)
        {
            return ECommandAction.Skipped;
        }

        // Strip the chat trigger prefix (!, ., /, backtick).
        if ("!./`".IndexOf(body[0]) >= 0)
        {
            body = body.Substring(1);
        }

        if (body.Length == 0)
        {
            return ECommandAction.Skipped;
        }

        var spaceIdx = body.IndexOf(' ');
        var cmd      = (spaceIdx < 0 ? body : body.Substring(0, spaceIdx))
                       .ToLowerInvariant();
        var arg      = (spaceIdx < 0 ? string.Empty : body.Substring(spaceIdx + 1))
                       .Trim();

        switch (cmd)
        {
            case "map":
            case "changemap":
                return HandleMap(client, arg);

            case "rtv":
                return HandleRtv(client);

            case "nominate":
            case "nom":
                return HandleNominate(client, arg);

            case "extend":
            case "ext":
                return HandleExtend(client);

            default:
                return ECommandAction.Skipped;
        }
    }

    // --- !map ---------------------------------------------------------------

    private ECommandAction HandleMap(IGameClient client, string arg)
    {
        // Three paths to grant !map access, in order:
        //   1. The SteamID is in MAP_ADMIN_STEAMIDS (env allowlist — failsafe).
        //   2. ModSharp's AdminManager has the user AND HasPermission succeeds
        //      on either `admin:map` or the `*` wildcard.
        //   3. Otherwise denied.
        var steamId64 = (ulong) client.SteamId;
        var envMatch  = _envAdminIds.Contains(steamId64);

#pragma warning disable CS0618
        var admin = _clientManager.FindAdmin(client.SteamId);
#pragma warning restore CS0618
        var permMatch = admin is not null
                        && (admin.HasPermission("admin:map")
                            || admin.HasPermission("*"));

        if (!envMatch && !permMatch)
        {
            _logger.LogInformation(
                "!map denied for {SteamId} (adminNull={Null} envAdmins={EnvCount})",
                client.SteamId, admin is null, _envAdminIds.Count);
            return ECommandAction.Handled;
        }

        _logger.LogInformation(
            "!map allowed for {SteamId} via {Via}",
            client.SteamId, envMatch ? "env" : "permissions");

        if (string.IsNullOrWhiteSpace(arg))
        {
            _logger.LogInformation("!map from {SteamId}: no arg", client.SteamId);
            return ECommandAction.Handled;
        }

        ChangeMap(arg);
        ResetVoteState(resetRotation: true);
        return ECommandAction.Handled;
    }

    // --- !rtv ---------------------------------------------------------------

    private ECommandAction HandleRtv(IGameClient client)
    {
        _rtvVoters.Add(client.SteamId);

        var connected = CountConnectedPlayers();
        var needed    = VotesNeeded(connected);

        _logger.LogInformation(
            "!rtv {SteamId}: {Have}/{Needed} (connected={Conn})",
            client.SteamId, _rtvVoters.Count, needed, connected);

        if (_rtvVoters.Count < needed)
        {
            return ECommandAction.Handled;
        }

        _logger.LogInformation("RTV threshold reached — forcing rotation now");
        _nextRotationAt = DateTime.UtcNow;
        return ECommandAction.Handled;
    }

    // --- !nominate ----------------------------------------------------------

    private ECommandAction HandleNominate(IGameClient client, string arg)
    {
        // Accept either a workshop publish file id or a map name. Name
        // resolution happens later in ChangeMap via ListWorkshopMaps.
        if (string.IsNullOrWhiteSpace(arg))
        {
            _logger.LogInformation(
                "!nominate from {SteamId}: no arg", client.SteamId);
            return ECommandAction.Handled;
        }

        _lastNomination = arg.Trim();
        _logger.LogInformation(
            "!nominate {SteamId} → {Map}", client.SteamId, _lastNomination);
        return ECommandAction.Handled;
    }

    // --- !extend ------------------------------------------------------------

    private ECommandAction HandleExtend(IGameClient client)
    {
        if (_extendsUsed >= _maxExtends)
        {
            _logger.LogInformation(
                "!extend {SteamId}: denied, max extends reached ({Used}/{Max})",
                client.SteamId, _extendsUsed, _maxExtends);
            return ECommandAction.Handled;
        }

        _extendVoters.Add(client.SteamId);

        var connected = CountConnectedPlayers();
        var needed    = VotesNeeded(connected);

        _logger.LogInformation(
            "!extend {SteamId}: {Have}/{Needed} (connected={Conn}, extends used={Used}/{Max})",
            client.SteamId, _extendVoters.Count, needed, connected, _extendsUsed, _maxExtends);

        if (_extendVoters.Count < needed)
        {
            return ECommandAction.Handled;
        }

        _nextRotationAt = _nextRotationAt.AddMinutes(_extendMinutes);
        _extendsUsed++;
        _extendVoters.Clear();
        _logger.LogInformation(
            "Extend passed — rotation pushed to {NextAt:u} ({Used}/{Max} used)",
            _nextRotationAt, _extendsUsed, _maxExtends);
        return ECommandAction.Handled;
    }

    // ------------------------------------------------------------------
    // Rotation
    // ------------------------------------------------------------------

    private void ScheduleTick()
    {
        // Self-rescheduling 10-second tick. ModSharp's PushTimer runs the
        // callback on the game thread, so calls into IModSharp / IClientManager
        // from OnTick are safe.
        _shared.GetModSharp().PushTimer(OnTick, 10.0f, GameTimerFlags.None);
    }

    private void OnTick()
    {
        // Reschedule immediately so a throw in DoScheduledRotation can't kill
        // the tick chain.
        ScheduleTick();
        try
        {
            if (DateTime.UtcNow >= _nextRotationAt)
            {
                DoScheduledRotation();
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "OnTick work failed");
        }
    }

    private void DoScheduledRotation()
    {
        var nextId = PickNextMap();
        if (nextId is null)
        {
            _logger.LogWarning(
                "Scheduled rotation: no maprotation.txt entries — retrying in {Min} min",
                _rotationMinutes);
            _nextRotationAt = DateTime.UtcNow.AddMinutes(_rotationMinutes);
            return;
        }

        _logger.LogInformation("Scheduled rotation → workshop {Map}", nextId);
        ChangeMap(nextId);
        ResetVoteState(resetRotation: true);
    }

    private void ChangeMap(string arg)
    {
        var modSharp = _shared.GetModSharp();

        // Numeric → treat as a workshop publish file id.
        if (long.TryParse(arg, out _))
        {
            _logger.LogInformation("host_workshop_map {WorkshopId}", arg);
            modSharp.ServerCommand($"host_workshop_map {arg}");
            return;
        }

        // Non-numeric → resolve against known workshop maps first. CS2 only
        // accepts `changelevel <name>` for maps whose vpk is actively mounted;
        // workshop maps need either `ds_workshop_changelevel <name>` (looks up
        // the map by its short name in the subscribed workshop items) or
        // `host_workshop_map <publishid>`. Try the workshop map list first so
        // "!map surf_kitsune" works without the admin needing to know IDs.
        try
        {
            var workshopMaps = modSharp.ListWorkshopMaps();
            _logger.LogInformation(
                "ListWorkshopMaps returned {Count} entries", workshopMaps.Count);

            (ulong PublishFileId, string Name)? hit = null;
            foreach (var m in workshopMaps)
            {
                if (!string.IsNullOrEmpty(m.Name)
                    && m.Name.Equals(arg, StringComparison.OrdinalIgnoreCase))
                {
                    hit = m;
                    break;
                }
            }

            if (hit is null)
            {
                foreach (var m in workshopMaps)
                {
                    if (!string.IsNullOrEmpty(m.Name)
                        && m.Name.Contains(arg, StringComparison.OrdinalIgnoreCase))
                    {
                        hit = m;
                        _logger.LogInformation(
                            "partial-matched {Arg} → {Name}", arg, m.Name);
                        break;
                    }
                }
            }

            if (hit is { } h)
            {
                _logger.LogInformation(
                    "resolved {Map} → workshop {Id}",
                    h.Name, h.PublishFileId);
                modSharp.ServerCommand($"host_workshop_map {h.PublishFileId}");
                return;
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "ListWorkshopMaps lookup failed");
        }

        // Last resort — a built-in or on-disk map. Will no-op if CS2 can't
        // find it, but we've exhausted the workshop path.
        if (modSharp.IsMapValid(arg))
        {
            _logger.LogInformation("ChangeLevel {Map}", arg);
            modSharp.ChangeLevel(arg);
            return;
        }

        _logger.LogWarning(
            "ChangeMap: {Arg} not a workshop map and not IsMapValid — giving up",
            arg);
    }

    private void ResetVoteState(bool resetRotation)
    {
        _rtvVoters.Clear();
        _extendVoters.Clear();
        _lastNomination = null;
        _extendsUsed    = 0;
        if (resetRotation)
        {
            _nextRotationAt = DateTime.UtcNow.AddMinutes(_rotationMinutes);
        }
    }

    // ------------------------------------------------------------------
    // Helpers
    // ------------------------------------------------------------------

    private int CountConnectedPlayers()
    {
        var count = 0;
        for (byte slot = 0; slot < 64; slot++)
        {
            var c = _clientManager.GetGameClient(new PlayerSlot(slot));
            if (c is not null && !c.IsFakeClient && c.IsAuthenticated)
            {
                count++;
            }
        }
        return count;
    }

    private static int VotesNeeded(int connected)
        => Math.Max(1, (connected / 2) + 1);

    private string? PickNextMap()
    {
        // Nomination wins regardless of rotation index.
        if (!string.IsNullOrWhiteSpace(_lastNomination))
        {
            var nom = _lastNomination;
            _lastNomination = null;
            return nom;
        }

        var entries = ReadRotationList();
        if (entries.Count == 0)
        {
            return null;
        }

        _rotationIndex = (_rotationIndex + 1) % entries.Count;
        return entries[_rotationIndex];
    }

    private List<string> ReadRotationList()
    {
        var path = Path.Combine(_sharpPath, "configs", "maprotation.txt");
        var result = new List<string>();
        if (!File.Exists(path))
        {
            return result;
        }

        try
        {
            foreach (var raw in File.ReadAllLines(path))
            {
                var line = raw.Trim();
                if (line.Length == 0 || line.StartsWith('#'))
                {
                    continue;
                }
                result.Add(line);
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to read maprotation.txt");
        }

        return result;
    }

    private static int ReadIntEnv(string name, int fallback)
    {
        var v = Environment.GetEnvironmentVariable(name);
        return int.TryParse(v, out var n) && n > 0 ? n : fallback;
    }
}
