/*
 * SurfMapCommand - native ModSharp module for map management.
 *
 * Commands:
 *   !map <id|name>         admin - immediate map change
 *   !rtv                   any   - rock-the-vote, triggers vote when threshold
 *   !nominate <id|name>    any   - add a map to the next vote's candidates
 *   !extend / !ext         any   - during a vote, vote for extend
 *   !maps / !maplist       any   - list rotation
 *   !addmap <id>           admin - add to rotation + download
 *   !removemap <id|name>   admin - remove from rotation
 *   !help / !commands      any   - list commands
 *   !1 .. !5               any   - vote for a candidate during vote phase
 *
 * Map cycle:
 *   1. Timer counts down MAP_ROTATION_MINUTES (default 30).
 *   2. At 2 minutes remaining, a VOTE starts: 5 random maps from the
 *      rotation + nominated maps are shown. Players type !1..!5 or !ext.
 *   3. After 30 seconds the vote closes. Winner announced.
 *   4. 30-second countdown, then map changes. If extend wins, deadline
 *      pushed by MAP_EXTEND_MINUTES.
 *   5. !rtv skips straight to step 2 when enough players vote.
 */

using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text;
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

namespace Cs2Surf.MapCommand;

public sealed class SurfMapCommand : IModSharpModule, IClientListener, IGameListener
{
    public string DisplayName   => "Cs2Surf.MapCommand";
    public string DisplayAuthor => "hwcopeland";

    private readonly ISharedSystem           _shared;
    private readonly ILogger<SurfMapCommand> _logger;
    private readonly IClientManager          _clientManager;
    private readonly string                  _sharpPath;

    // --- RTV state ---
    private readonly HashSet<SteamID> _rtvVoters = [];

    // --- Nominations (persist until consumed by a vote) ---
    private readonly List<string> _nominations = [];

    // --- Vote state ---
    private enum VotePhase { None, Voting, Countdown }
    private VotePhase         _votePhase;
    private DateTime          _voteDeadline;
    private List<string>      _voteCandidates  = [];  // index 0..N-1 = maps, last = "extend"
    private Dictionary<SteamID, int> _votes     = [];  // steamid → candidate index
    private string?           _voteWinner;
    private bool              _countdown10sAnnounced;

    // --- Rotation state ---
    private DateTime _nextRotationAt;
    private int      _rotationIndex;
    private int      _extendsUsed;

    // --- Player count for Loki ---
    private int _lastPlayerCount = -1;

    // --- Config ---
    private int _rotationMinutes = 30;
    private int _extendMinutes   = 15;
    private int _maxExtends      = 3;

    // Env-configured admin allowlist.
    private readonly HashSet<ulong> _envAdminIds = [];

    // MySQL connection string for rank/leaderboard queries.
    private string _mysqlConnStr = "";

    // ─── Rank definitions (color code, name, min points) ───────────────
    private static readonly (string Color, string Name, int MinPoints)[] Ranks =
    [
        ("\x08", "Unranked",      0),
        ("\x07", "First Blood",   150),
        ("\x06", "Impressive",    750),
        ("\x02", "Rampage",       1_500),
        ("\x07", "Savage",        2_500),
        ("\x09", "Unstoppable",   4_000),
        ("\x02", "Monster",       6_000),
        ("\x04", "Relentless",    9_000),
        ("\x0E", "Wicked",        13_000),
        ("\x0F", "Ludicrous",     18_000),
        ("\x07", "Dominating",    25_000),
        ("\x02", "Menace",        33_000),
        ("\x0B", "Demon",         42_000),
        ("\x09", "Legendary",     55_000),
        ("\x04", "Apex",          70_000),
        ("\x0E", "Combowhore",    88_000),
        ("\x0D", "Transcendent",  110_000),
        ("\x0B", "Immortal",      135_000),
        ("\x02", "Holy Shit",     165_000),
        ("\x07", "Godlike",       200_000),
    ];

    private static (string Color, string Name) GetRank(int points)
    {
        for (int i = Ranks.Length - 1; i >= 0; i--)
        {
            if (points >= Ranks[i].MinPoints)
                return (Ranks[i].Color, Ranks[i].Name);
        }
        return (Ranks[0].Color, Ranks[0].Name);
    }

    // Cache player points to avoid querying MySQL on every chat message.
    private readonly Dictionary<ulong, int> _playerPointsCache = [];

    private string GetPlayerRankTitle(SteamID steamId)
    {
        var points = GetPlayerPoints(steamId);
        var (color, name) = GetRank(points);
        return $"{color}[{name}]";
    }

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
            _logger.LogInformation("MySQL configured for rank queries");
        }
        else
        {
            _logger.LogWarning("MYSQL_HOST/MYSQL_USER not set - !rank and !lb will not work");
        }

        _nextRotationAt = DateTime.UtcNow.AddMinutes(_rotationMinutes);
        _clientManager.InstallClientListener(this);
        _shared.GetModSharp().InstallGameListener(this);
        ScheduleTick();

        _logger.LogInformation(
            "SurfMapCommand loaded - rotation={Rot}m extend={Ext}m maxExt={Max} envAdmins={Adm}",
            _rotationMinutes, _extendMinutes, _maxExtends, _envAdminIds.Count);
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

    void IGameListener.OnServerActivate()
    {
        _logger.LogInformation("OnServerActivate - resetting state");
        ScheduleTick();
        _playerPointsCache.Clear();
        _rtvVoters.Clear();
        _nominations.Clear();
        _votes.Clear();
        _voteCandidates.Clear();
        _votePhase            = VotePhase.None;
        _voteWinner           = null;
        _extendsUsed          = 0;
        _countdown10sAnnounced = false;
        _nextRotationAt       = DateTime.UtcNow.AddMinutes(_rotationMinutes);
    }

    // ─── Chat ──────────────────────────────────────────────────────────

    public void OnClientPutInServer(IGameClient client)
    {
        if (client.IsFakeClient) return;
        _logger.LogInformation("CONNECT {Id} ({Name})", client.SteamId, client.Name);

        // Set clan tag to rank title in tab menu / scoreboard.
        try
        {
            var (_, rankName) = GetRank(GetPlayerPoints(client.SteamId));
            if (client.GetPlayerController() is { } ctrl)
            {
                ctrl.SetClanTag($"[{rankName}]");
            }
        }
        catch { }
    }

    private int GetPlayerPoints(SteamID steamId)
    {
        var id = (ulong)steamId;
        if (_playerPointsCache.TryGetValue(id, out var cached)) return cached;
        if (string.IsNullOrEmpty(_mysqlConnStr)) return 0;
        try
        {
            using var conn = new MySqlConnection(_mysqlConnStr);
            conn.Open();
            using var cmd = new MySqlCommand("SELECT Points FROM surf_players WHERE SteamId = @sid", conn);
            cmd.Parameters.AddWithValue("@sid", (long)id);
            var result = cmd.ExecuteScalar();
            var pts = result is not null ? Convert.ToInt32(result) : 0;
            _playerPointsCache[id] = pts;
            return pts;
        }
        catch { return 0; }
    }

    public void OnClientDisconnecting(IGameClient client, NetworkDisconnectionReason reason)
    {
        if (!client.IsFakeClient)
            _logger.LogInformation("DISCONNECT {Id} ({Name}) reason={R}", client.SteamId, client.Name, reason);
    }

    public ECommandAction OnClientSayCommand(IGameClient client, bool teamOnly, bool isCommand,
                                             string commandName, string message)
    {
        var rawMsg = (message ?? "").TrimStart();
        if (rawMsg.Length > 0 && !client.IsFakeClient)
        {
            _logger.LogInformation("CHAT {Id} ({Name}): {Msg}", client.SteamId, client.Name, rawMsg);

            // Re-broadcast with rank title prefix (suppress original via Handled).
            // Only for regular chat, not commands (commands get handled below).
            if (rawMsg.Length > 0 && "!./`".IndexOf(rawMsg[0]) < 0)
            {
                var rankTitle = GetPlayerRankTitle(client.SteamId);
                _shared.GetModSharp().PrintToChatAll(
                    $" {rankTitle} \x01{client.Name}\x08: {rawMsg}");
                return ECommandAction.Handled;
            }
        }

        // Check for trigger prefix.
        var body = rawMsg;
        if (body.Length == 0 || "!./`".IndexOf(body[0]) < 0)
            return ECommandAction.Skipped;
        body = body.Substring(1);
        if (body.Length == 0)
            return ECommandAction.Skipped;

        var spaceIdx = body.IndexOf(' ');
        var cmd = (spaceIdx < 0 ? body : body.Substring(0, spaceIdx)).ToLowerInvariant();
        var arg = (spaceIdx < 0 ? "" : body.Substring(spaceIdx + 1)).Trim();

        // Vote number commands during voting phase.
        if (_votePhase == VotePhase.Voting && cmd.Length == 1 && char.IsDigit(cmd[0]))
        {
            return HandleVoteChoice(client, cmd[0] - '0');
        }

        switch (cmd)
        {
            case "map": case "changemap":       return HandleMap(client, arg);
            case "testsound":                    return HandleTestSound(client, arg);
            case "testweapon":                   return HandleTestWeapon(client, arg);
            case "rtv":                          return HandleRtv(client);
            case "nominate": case "nom":         return HandleNominate(client, arg);
            case "extend": case "ext":
                return _votePhase == VotePhase.Voting
                    ? HandleVoteChoice(client, _voteCandidates.Count - 1)  // extend is last
                    : ECommandAction.Skipped;
            case "maps": case "maplist":         return HandleMaps(client);
            case "addmap":                       return HandleAddMap(client, arg);
            case "removemap": case "delmap":     return HandleRemoveMap(client, arg);
            case "help": case "commands": case "h": return HandleHelp(client);
            case "rank":                         return HandleRank(client);
            case "lb": case "leaderboard": case "top": return HandleLeaderboard(client);
            default:                             return ECommandAction.Skipped;
        }
    }

    // ─── Timer ───────────────────────────────────��─────────────────────

    private void ScheduleTick()
    {
        _shared.GetModSharp().PushTimer(OnTick, 10.0f, GameTimerFlags.None);
    }

    private void OnTick()
    {
        ScheduleTick();
        try
        {
            var players = CountConnectedPlayers();
            if (players != _lastPlayerCount)
            {
                _logger.LogInformation("PLAYERS {C}", players);
                _lastPlayerCount = players;
            }

            var now = DateTime.UtcNow;

            switch (_votePhase)
            {
                case VotePhase.Voting:
                    if (now >= _voteDeadline) CloseVote();
                    break;

                case VotePhase.Countdown:
                    var rem = (_voteDeadline - now).TotalSeconds;
                    if (rem <= 10 && !_countdown10sAnnounced)
                    {
                        Announce($" \x04[surf] \x01Map changing to \x09{ResolveDisplayName(_voteWinner!)} \x01in \x0710 seconds\x01...");
                        _countdown10sAnnounced = true;
                    }
                    if (now >= _voteDeadline)
                    {
                        _logger.LogInformation("Countdown done → {Map}", _voteWinner);
                        ChangeMap(_voteWinner!);
                        _votePhase  = VotePhase.None;
                        _voteWinner = null;
                    }
                    break;

                case VotePhase.None:
                    // Start a vote 2 minutes before rotation deadline.
                    var until = (_nextRotationAt - now).TotalSeconds;
                    if (until <= 120) StartVote();
                    break;
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "OnTick failed");
        }
    }

    // ─── Vote system ───────────────────────────────────────────────────

    private void StartVote()
    {
        _votePhase    = VotePhase.Voting;
        _voteDeadline = DateTime.UtcNow.AddSeconds(120); // 2 minute vote
        _votes.Clear();

        // Build candidates: 5 random maps + any nominations on top.
        var all        = ReadRotationList();
        var candidates = new List<string>();

        // Add 5 random maps from rotation.
        var rng = new Random();
        var shuffled = all.OrderBy(_ => rng.Next()).ToList();
        foreach (var entry in shuffled)
        {
            if (candidates.Count >= 5) break;
            candidates.Add(entry);
        }

        // Add nominations on top (not counted against the 5).
        foreach (var nom in _nominations)
        {
            if (!candidates.Contains(nom)) candidates.Add(nom);
        }
        _nominations.Clear();

        // Add "Extend" as the last option (if extends remaining).
        if (_extendsUsed < _maxExtends)
            candidates.Add("extend");

        _voteCandidates = candidates;

        // Query DB for record info + stage count for each candidate.
        var mapInfo = new Dictionary<string, (bool hasSR, int stages)>();
        if (!string.IsNullOrEmpty(_mysqlConnStr))
        {
            try
            {
                using var conn = new MySqlConnection(_mysqlConnStr);
                conn.Open();
                foreach (var c in candidates.Where(c => c != "extend"))
                {
                    var name = ResolveDisplayName(c);
                    using var cmd = new MySqlCommand(
                        @"SELECT
                            (SELECT COUNT(*) FROM surf_player_best_runs r JOIN surf_maps m ON m.MapId = r.MapId WHERE m.File = @f AND r.RunType = 0 AND r.Style = 0 AND r.Track = 0) AS has_sr,
                            (SELECT COALESCE(MAX(r2.Stage), 0) FROM surf_player_best_runs r2 JOIN surf_maps m2 ON m2.MapId = r2.MapId WHERE m2.File = @f AND r2.RunType = 1) AS stages",
                        conn);
                    cmd.Parameters.AddWithValue("@f", name);
                    using var reader = cmd.ExecuteReader();
                    if (reader.Read())
                        mapInfo[name] = (reader.GetInt32(0) > 0, reader.GetInt32(1));
                    else
                        mapInfo[name] = (false, 0);
                }
            }
            catch { }
        }

        var tiers = ReadMapTiers();

        Announce("========= VOTE: Next Map =========");
        for (int i = 0; i < candidates.Count; i++)
        {
            if (candidates[i] == "extend")
            {
                Announce($"  [{i + 1}] Extend (+{_extendMinutes}min)");
                continue;
            }
            var name = ResolveDisplayName(candidates[i]);
            var (hasSR, stages) = mapInfo.GetValueOrDefault(name, (false, 0));
            var tier = tiers.GetValueOrDefault(name, 0);
            var tierTag = tier > 0 ? $"[T{tier}]" : "";
            var stageTag = stages > 0 ? $"S{stages}" : "L";
            var recordTag = hasSR ? "" : " *";
            Announce($"  [{i + 1}] {name} {tierTag} {stageTag}{recordTag}");
        }
        Announce($"Type !1 - !{candidates.Count} to vote. 2 minutes!");
        Announce("==================================");
        _logger.LogInformation("Vote started with {N} candidates", candidates.Count);
    }

    private ECommandAction HandleVoteChoice(IGameClient client, int num)
    {
        var idx = num - 1;
        if (idx < 0 || idx >= _voteCandidates.Count)
            return ECommandAction.Handled;

        _votes[client.SteamId] = idx;
        var label = _voteCandidates[idx] == "extend"
            ? "Extend"
            : ResolveDisplayName(_voteCandidates[idx]);
        Reply(client, $"[surf] Voted for {label}");
        return ECommandAction.Handled;
    }

    private void CloseVote()
    {
        // Tally.
        var tally = new int[_voteCandidates.Count];
        foreach (var v in _votes.Values)
        {
            if (v >= 0 && v < tally.Length) tally[v]++;
        }

        int winIdx = 0;
        for (int i = 1; i < tally.Length; i++)
        {
            if (tally[i] > tally[winIdx]) winIdx = i;
        }

        var winner = _voteCandidates[winIdx];
        _votes.Clear();

        if (winner == "extend")
        {
            _extendsUsed++;
            _nextRotationAt = DateTime.UtcNow.AddMinutes(_extendMinutes);
            _votePhase = VotePhase.None;
            Announce($" \x04[surf] \x0AExtend wins! \x01Map extended by \x09{_extendMinutes} \x01min \x08({_extendsUsed}/{_maxExtends})");
            _logger.LogInformation("Vote result: extend ({Used}/{Max})", _extendsUsed, _maxExtends);
            return;
        }

        // Start countdown.
        _voteWinner            = winner;
        _votePhase             = VotePhase.Countdown;
        _voteDeadline          = DateTime.UtcNow.AddSeconds(30);
        _countdown10sAnnounced = false;
        var display = ResolveDisplayName(winner);
        Announce($" \x04[surf] \x09{display} \x01wins! Changing in \x0930 seconds\x01...");
        _logger.LogInformation("Vote result: {Map} ({Display})", winner, display);
    }

    // ─── Commands ──────────────────────────────────────────────────────

    private ECommandAction HandleTestSound(IGameClient client, string arg)
    {
        if (!RequireAdmin(client, "!testsound")) return ECommandAction.Handled;

        var method = (arg ?? "help").Trim();

        Reply(client, $"[surf] Testing sound: {method}");
        _logger.LogInformation("TESTSOUND {M} for {Id}", method, client.SteamId);

        try
        {
            // Test different sound paths + APIs
            switch (method)
            {
                // Built-in CS2 sounds (should definitely work if API is correct)
                case "builtin1":
                    client.ExecuteStringCommand("play sounds/ui/panorama/case_awarded_1_uncommon_01");
                    Reply(client, "[surf] builtin case_awarded sent");
                    break;
                case "builtin2":
                    client.ExecuteStringCommand("play sounds/music/valve_csgo_03/startround_01");
                    Reply(client, "[surf] builtin startround sent");
                    break;
                case "builtin3":
                    client.ExecuteStringCommand("play sounds/ui/panorama/music_mainmenu_01");
                    Reply(client, "[surf] builtin mainmenu sent");
                    break;

                // Sound manager with built-in events
                case "event1":
                    _shared.GetSoundManager().StartSoundEvent("UIPanorama.case_awarded_uncommon");
                    Reply(client, "[surf] event case_awarded_uncommon");
                    break;
                case "event2":
                    _shared.GetSoundManager().StartSoundEvent("Music.MVPPreviewMusic");
                    Reply(client, "[surf] event MVPPreviewMusic");
                    break;

                // Our custom sound with different path formats
                case "custom1":
                    client.ExecuteStringCommand("play cs2/quakesounds/default/godlike");
                    Reply(client, "[surf] custom path 1");
                    break;
                case "custom2":
                    client.ExecuteStringCommand("play sounds/surf/godlike");
                    Reply(client, "[surf] custom path 2");
                    break;
                case "custom3":
                    client.ExecuteStringCommand("play sound/surf/godlike");
                    Reply(client, "[surf] custom path 3");
                    break;

                // Pawn emit with built-in
                case "emit":
                    if (client.GetPlayerController()?.GetPlayerPawn() is { } p)
                    {
                        p.EmitSound("Player.DamageKevlar");
                        Reply(client, "[surf] EmitSound Player.DamageKevlar");
                    }
                    break;

                default:
                    Reply(client, "[surf] !testsound <test>");
                    Reply(client, "  builtin1 builtin2 builtin3");
                    Reply(client, "  event1 event2");
                    Reply(client, "  custom1 custom2 custom3");
                    Reply(client, "  emit");
                    break;
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "TESTSOUND method {M} threw", method);
            Reply(client, $"[surf] Method {method} threw: {ex.Message}");
        }

        return ECommandAction.Handled;
    }

    private ECommandAction HandleTestWeapon(IGameClient client, string arg)
    {
        if (!RequireAdmin(client, "!testweapon")) return ECommandAction.Handled;

        var method = (arg ?? "1").Trim();
        var ctrl = client.GetPlayerController();
        var pawn = ctrl?.GetPlayerPawn();
        if (pawn is null || !pawn.IsAlive)
        {
            Reply(client, "[surf] Must be alive");
            return ECommandAction.Handled;
        }

        Reply(client, $"[surf] Testing weapon method {method}...");
        _logger.LogInformation("TESTWEAPON method={M} for {Id}", method, client.SteamId);

        try
        {
            switch (method)
            {
                case "1": // Strip all + give knife
                    pawn.RemoveAllItems(true);
                    _shared.GetModSharp().InvokeFrameAction(() =>
                    {
                        if (pawn.IsAlive) pawn.GiveNamedItem("weapon_knife");
                    });
                    Reply(client, "[surf] Stripped + knife given (next frame)");
                    break;
                case "2": // Give USP
                    pawn.GiveNamedItem("weapon_usp_silencer");
                    Reply(client, "[surf] USP given");
                    break;
                case "3": // Give Scout
                    pawn.GiveNamedItem("weapon_ssg08");
                    Reply(client, "[surf] Scout given");
                    break;
                case "4": // Full loadout: strip + knife + usp + scout
                    pawn.RemoveAllItems(true);
                    _shared.GetModSharp().InvokeFrameAction(() =>
                    {
                        if (!pawn.IsAlive) return;
                        pawn.GiveNamedItem("weapon_knife");
                        pawn.GiveNamedItem("weapon_usp_silencer");
                        pawn.GiveNamedItem("weapon_ssg08");
                    });
                    Reply(client, "[surf] Full loadout (next frame)");
                    break;
                default:
                    Reply(client, "[surf] Usage: !testweapon <1-4>");
                    Reply(client, "  1=strip+knife  2=give usp  3=give scout  4=full loadout");
                    break;
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "TESTWEAPON method {M} threw", method);
            Reply(client, $"[surf] Method {method} threw: {ex.Message}");
        }

        return ECommandAction.Handled;
    }

    private ECommandAction HandleMap(IGameClient client, string arg)
    {
        if (!RequireAdmin(client, "!map")) return ECommandAction.Handled;
        if (string.IsNullOrWhiteSpace(arg))
        {
            Reply(client, "[surf] Usage: !map <id|name>");
            return ECommandAction.Handled;
        }
        _logger.LogInformation("!map allowed for {Id} via {Via}", client.SteamId,
            _envAdminIds.Contains((ulong)client.SteamId) ? "env" : "perms");
        ChangeMap(arg);
        ResetVoteState();
        return ECommandAction.Handled;
    }

    private ECommandAction HandleRtv(IGameClient client)
    {
        if (_votePhase != VotePhase.None)
        {
            Reply(client, "[surf] A vote is already in progress.");
            return ECommandAction.Handled;
        }
        _rtvVoters.Add(client.SteamId);
        var needed = VotesNeeded(CountConnectedPlayers());
        Announce($" \x04[surf] \x09{_rtvVoters.Count}/{needed} \x01players want to rock the vote.");
        if (_rtvVoters.Count >= needed)
        {
            _rtvVoters.Clear();
            Announce(" \x04[surf] \x09RTV passed!");
            StartVote();
        }
        return ECommandAction.Handled;
    }

    private ECommandAction HandleNominate(IGameClient client, string arg)
    {
        if (string.IsNullOrWhiteSpace(arg))
        {
            Reply(client, "[surf] Usage: !nominate <id|name>");
            return ECommandAction.Handled;
        }
        // Resolve name → id if possible.
        var resolved = arg.Trim();
        if (!ulong.TryParse(resolved, out _))
        {
            var names = ReadMapNames();
            if (names.TryGetValue(resolved.ToLowerInvariant(), out var id))
                resolved = id.ToString();
        }
        if (!_nominations.Contains(resolved))
            _nominations.Add(resolved);
        Announce($" \x04[surf] \x09{ResolveDisplayName(resolved)} \x01nominated for next vote.");
        return ECommandAction.Handled;
    }

    private ECommandAction HandleMaps(IGameClient client)
    {
        var rotation = ReadRotationList();
        var names    = ReadMapNames();
        var idToName = new Dictionary<ulong, string>();
        foreach (var (n, id) in names) idToName.TryAdd(id, n);

        if (rotation.Count == 0) { Reply(client, "[surf] Rotation is empty."); return ECommandAction.Handled; }

        Reply(client, $"[surf] {rotation.Count} maps in rotation:");
        var line = new StringBuilder();
        var c = 0;
        foreach (var entry in rotation)
        {
            string display = entry;
            if (ulong.TryParse(entry, out var id) && idToName.TryGetValue(id, out var n)) display = n;
            if (c > 0) line.Append(", ");
            line.Append(display);
            if (++c >= 5) { Reply(client, "[surf] " + line); line.Clear(); c = 0; }
        }
        if (line.Length > 0) Reply(client, "[surf] " + line);
        return ECommandAction.Handled;
    }

    private ECommandAction HandleAddMap(IGameClient client, string arg)
    {
        if (!RequireAdmin(client, "!addmap")) return ECommandAction.Handled;
        if (string.IsNullOrWhiteSpace(arg) || !ulong.TryParse(arg.Trim(), out var workshopId))
        {
            Reply(client, "[surf] Usage: !addmap <workshopid>");
            return ECommandAction.Handled;
        }
        var trimmed = arg.Trim();
        var path = Path.Combine(_sharpPath, "configs", "maprotation.txt");
        try
        {
            var lines = File.Exists(path) ? File.ReadAllLines(path).ToList() : [];
            if (lines.Any(l => l.Trim() == trimmed))
            { Reply(client, $"[surf] {trimmed} already in rotation."); return ECommandAction.Handled; }
            lines.Add(trimmed);
            File.WriteAllLines(path, lines);
            Reply(client, $"[surf] Added {trimmed}. Downloading...");
        }
        catch (Exception ex) { _logger.LogError(ex, "!addmap failed"); Reply(client, "[surf] Failed."); return ECommandAction.Handled; }
        EnsureSubscribed(workshopId);
        _shared.GetModSharp().ServerCommand($"host_workshop_map {workshopId}");
        return ECommandAction.Handled;
    }

    private ECommandAction HandleRemoveMap(IGameClient client, string arg)
    {
        if (!RequireAdmin(client, "!removemap")) return ECommandAction.Handled;
        if (string.IsNullOrWhiteSpace(arg)) { Reply(client, "[surf] Usage: !removemap <id|name>"); return ECommandAction.Handled; }
        var trimmed = arg.Trim();
        if (!ulong.TryParse(trimmed, out _))
        {
            var names = ReadMapNames();
            if (names.TryGetValue(trimmed.ToLowerInvariant(), out var id)) trimmed = id.ToString();
        }
        var path = Path.Combine(_sharpPath, "configs", "maprotation.txt");
        try
        {
            if (!File.Exists(path)) { Reply(client, "[surf] Rotation file missing."); return ECommandAction.Handled; }
            var lines = File.ReadAllLines(path).ToList();
            var before = lines.Count;
            lines.RemoveAll(l => l.Trim() == trimmed);
            if (lines.Count == before) { Reply(client, $"[surf] {arg} not found."); return ECommandAction.Handled; }
            File.WriteAllLines(path, lines);
            Reply(client, $"[surf] Removed {arg} ({lines.Count} maps).");
        }
        catch (Exception ex) { _logger.LogError(ex, "!removemap failed"); Reply(client, "[surf] Failed."); }
        return ECommandAction.Handled;
    }

    private ECommandAction HandleHelp(IGameClient client)
    {
        Reply(client, " \x04===== W4de Surf Commands =====");
        Reply(client, " \x09Timer");
        Reply(client, "  \x09!r \x08- restart  \x09!s \x08- stage select  \x09!b \x08- bonus");
        Reply(client, "  \x09!stop \x08- stop timer  \x09!pause \x08- pause  \x09!resume \x08- resume");
        Reply(client, "  \x09!nc \x08- noclip  \x09!spec \x08- spectate");
        Reply(client, " \x09Records");
        Reply(client, "  \x09!sr \x08- server record  \x09!ssr \x08- stage SR  \x09!bsr \x08- bonus SR");
        Reply(client, "  \x09!pb \x08- personal best  \x09!spb \x08- stage PB  \x09!bpb \x08- bonus PB");
        Reply(client, "  \x09!cpr \x08- checkpoint comparison  \x09!recent \x08- recent runs");
        Reply(client, " \x09Stats");
        Reply(client, "  \x09!rank \x08- your rank  \x09!top \x08- leaderboard  \x09!stats \x08- player stats");
        Reply(client, "  \x09!profile \x08- profile  \x09!playtime \x08- play time  \x09!mi \x08- map info");
        Reply(client, " \x09Practice");
        Reply(client, "  \x09!save \x08- save position  \x09!tele \x08- teleport back  \x09!clearcp \x08- clear");
        Reply(client, "  \x09!knife \x08- give knife  \x09!usp \x08- give usp  \x09!glock \x08- give glock");
        Reply(client, " \x09Map Vote");
        Reply(client, "  \x09!rtv \x08- rock the vote  \x09!nominate \x08- nominate map  \x09!extend \x08- extend");
        Reply(client, "  \x09!maps \x08- map list  \x09!map \x01<name> \x08- change map (admin)");
        Reply(client, " \x04=============================");
        return ECommandAction.Handled;
    }

    // ─── Ranks ─────────────────────────────────────────────────────────

    private ECommandAction HandleRank(IGameClient client)
    {
        if (string.IsNullOrEmpty(_mysqlConnStr))
        {
            Reply(client, "\x07[surf] Ranks not available (DB not configured)");
            return ECommandAction.Handled;
        }

        try
        {
            var steamId = (ulong)client.SteamId;
            using var conn = new MySqlConnection(_mysqlConnStr);
            conn.Open();

            // Get player points + position
            using var cmd = new MySqlCommand(
                @"SELECT p.Name, p.Points,
                  (SELECT COUNT(*) + 1 FROM surf_players p2 WHERE p2.Points > p.Points) AS position,
                  (SELECT COUNT(*) FROM surf_players WHERE Points > 0) AS total
                  FROM surf_players p WHERE p.SteamId = @sid",
                conn);
            cmd.Parameters.AddWithValue("@sid", (long)steamId);

            using var reader = cmd.ExecuteReader();
            if (reader.Read())
            {
                var name   = reader.GetString("Name");
                var points = reader.GetInt32("Points");
                var pos    = reader.GetInt64("position");
                var total  = reader.GetInt64("total");
                var (color, rankName) = GetRank(points);

                Reply(client, $"\x04======= Your Rank =======");
                Reply(client, $" \x01{name}");
                Reply(client, $" Rank: {color}{rankName}");
                Reply(client, $" \x09{points:N0} \x01points \x08(#{pos} of {total})");

                // Next rank
                for (int i = 0; i < Ranks.Length - 1; i++)
                {
                    if (points < Ranks[i + 1].MinPoints)
                    {
                        var needed = Ranks[i + 1].MinPoints - points;
                        Reply(client, $" \x08Next: {Ranks[i + 1].Color}{Ranks[i + 1].Name} \x08({needed:N0} pts)");
                        break;
                    }
                }
                Reply(client, $"\x04=========================");
            }
            else
            {
                Reply(client, "\x08[surf] No rank data yet - complete a map!");
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "HandleRank failed");
            Reply(client, "\x07[surf] Error loading rank");
        }

        return ECommandAction.Handled;
    }

    private ECommandAction HandleLeaderboard(IGameClient client)
    {
        if (string.IsNullOrEmpty(_mysqlConnStr))
        {
            Reply(client, "\x07[surf] Leaderboard not available (DB not configured)");
            return ECommandAction.Handled;
        }

        try
        {
            using var conn = new MySqlConnection(_mysqlConnStr);
            conn.Open();

            using var cmd = new MySqlCommand(
                "SELECT Name, Points FROM surf_players WHERE Points > 0 ORDER BY Points DESC LIMIT 10",
                conn);
            using var reader = cmd.ExecuteReader();

            Reply(client, "\x04======= Leaderboard =======");
            int pos = 1;
            while (reader.Read())
            {
                var name   = reader.GetString("Name");
                var points = reader.GetInt32("Points");
                var (color, rankName) = GetRank(points);

                var medal = pos switch
                {
                    1 => "\x09#1",
                    2 => "\x08#2",
                    3 => "\x02#3",
                    _ => $"\x01#{pos}",
                };
                Reply(client, $" {medal} {color}[{rankName}] \x01{name} \x08- \x09{points:N0} pts");
                pos++;
            }
            Reply(client, "\x04===========================");
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "HandleLeaderboard failed");
            Reply(client, "\x07[surf] Error loading leaderboard");
        }

        return ECommandAction.Handled;
    }

    // ─── Map change ────────────────────────────────────────────────────

    private void ChangeMap(string arg)
    {
        var modSharp = _shared.GetModSharp();
        if (ulong.TryParse(arg, out var workshopId))
        {
            EnsureSubscribed(workshopId);
            _logger.LogInformation("host_workshop_map {Id}", workshopId);
            modSharp.ServerCommand($"host_workshop_map {workshopId}");
            return;
        }
        var names = ReadMapNames();
        if (names.TryGetValue(arg.ToLowerInvariant(), out var id))
        {
            EnsureSubscribed(id);
            _logger.LogInformation("resolved {Name} → {Id}", arg, id);
            modSharp.ServerCommand($"host_workshop_map {id}");
            return;
        }
        _logger.LogInformation("ds_workshop_changelevel {Name}", arg);
        modSharp.ServerCommand($"ds_workshop_changelevel {arg}");
    }

    private void EnsureSubscribed(ulong workshopId)
    {
        var path = Path.Combine(_shared.GetModSharp().GetGamePath(), "subscribed_file_ids.txt");
        try
        {
            var idStr = workshopId.ToString();
            var existing = new HashSet<string>();
            if (File.Exists(path))
                foreach (var line in File.ReadAllLines(path))
                    if (line.Trim().Length > 0) existing.Add(line.Trim());
            if (existing.Contains(idStr)) return;
            File.AppendAllText(path, Environment.NewLine + idStr + Environment.NewLine);
            _logger.LogInformation("Subscribed {Id}", idStr);
        }
        catch (Exception ex) { _logger.LogError(ex, "EnsureSubscribed failed"); }
    }

    private void ResetVoteState()
    {
        _rtvVoters.Clear();
        _nominations.Clear();
        _votes.Clear();
        _voteCandidates.Clear();
        _votePhase  = VotePhase.None;
        _voteWinner = null;
        _extendsUsed = 0;
        _nextRotationAt = DateTime.UtcNow.AddMinutes(_rotationMinutes);
    }

    // ─── Helpers ───────────────────────────────────────────────────────

    private void Reply(IGameClient client, string msg)
    {
        try { client.GetPlayerController()?.GetPlayerPawn()?.Print(HudPrintChannel.Chat, msg); }
        catch { }
    }

    private void Announce(string msg)
        => _shared.GetModSharp().PrintToChatAll(msg);

    private bool RequireAdmin(IGameClient client, string action)
    {
        if (_envAdminIds.Contains((ulong)client.SteamId)) return true;
#pragma warning disable CS0618
        var admin = _clientManager.FindAdmin(client.SteamId);
#pragma warning restore CS0618
        if (admin is not null && (admin.HasPermission("admin:map") || admin.HasPermission("*")))
            return true;
        Reply(client, $"[surf] {action}: admin required.");
        return false;
    }

    private int CountConnectedPlayers()
    {
        var count = 0;
        for (byte slot = 0; slot < 64; slot++)
        {
            var c = _clientManager.GetGameClient(new PlayerSlot(slot));
            if (c is not null && !c.IsFakeClient && c.IsAuthenticated) count++;
        }
        return count;
    }

    private static int VotesNeeded(int connected) => Math.Max(1, (connected / 2) + 1);

    private string ResolveDisplayName(string arg)
    {
        if (!ulong.TryParse(arg, out var id)) return arg;
        foreach (var (name, mapId) in ReadMapNames())
            if (mapId == id) return name;
        return arg;
    }

    private string? PickNextMap()
    {
        var entries = ReadRotationList();
        if (entries.Count == 0) return null;
        _rotationIndex = (_rotationIndex + 1) % entries.Count;
        return entries[_rotationIndex];
    }

    private List<string> ReadRotationList()
    {
        var path = Path.Combine(_sharpPath, "configs", "maprotation.txt");
        var result = new List<string>();
        if (!File.Exists(path)) return result;
        try
        {
            foreach (var raw in File.ReadAllLines(path))
            {
                var line = raw.Trim();
                if (line.Length > 0 && !line.StartsWith('#')) result.Add(line);
            }
        }
        catch (Exception ex) { _logger.LogError(ex, "ReadRotationList failed"); }
        return result;
    }

    private Dictionary<string, ulong> ReadMapNames()
    {
        var path = Path.Combine(_sharpPath, "configs", "mapnames.txt");
        var result = new Dictionary<string, ulong>(StringComparer.OrdinalIgnoreCase);
        if (!File.Exists(path)) return result;
        try
        {
            foreach (var raw in File.ReadAllLines(path))
            {
                var line = raw.Trim();
                if (line.Length == 0 || line.StartsWith('#')) continue;
                var parts = line.Split([' ', '\t'], 2, StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries);
                if (parts.Length == 2 && ulong.TryParse(parts[1], out var id)) result[parts[0]] = id;
            }
        }
        catch (Exception ex) { _logger.LogError(ex, "ReadMapNames failed"); }
        return result;
    }

    private Dictionary<string, int> ReadMapTiers()
    {
        var path = Path.Combine(_sharpPath, "configs", "maptiers.txt");
        var result = new Dictionary<string, int>(StringComparer.OrdinalIgnoreCase);
        if (!File.Exists(path)) return result;
        try
        {
            foreach (var raw in File.ReadAllLines(path))
            {
                var line = raw.Trim();
                if (line.Length == 0 || line.StartsWith('#')) continue;
                var parts = line.Split([' ', '\t'], 2, StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries);
                if (parts.Length == 2 && int.TryParse(parts[1], out var tier)) result[parts[0]] = tier;
            }
        }
        catch (Exception ex) { _logger.LogError(ex, "ReadMapTiers failed"); }
        return result;
    }

    private static int ReadIntEnv(string name, int fallback)
    {
        var v = Environment.GetEnvironmentVariable(name);
        return int.TryParse(v, out var n) && n > 0 ? n : fallback;
    }
}
