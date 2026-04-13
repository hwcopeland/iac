/*
 * SurfMapCommand — native ModSharp module providing !map, !rtv, !nominate.
 *
 * Hooks IClientListener.OnClientSayCommand and handles surf-server admin
 * commands + RTV vote without any Tnms / CS# / MCS baggage. ~200 lines,
 * one NuGet dep (ModSharp.Sharp.Shared).
 *
 * Commands:
 *   !map <workshopid_or_name>   admin:map — change map immediately
 *   !rtv                        any player — vote to rock-the-vote;
 *                               when > 50% of connected players have
 *                               RTV'd, triggers next-in-rotation
 *   !nominate <workshopid>      any player — adds a workshop id to the
 *                               RTV queue (MVP: just stores last nom)
 *
 * Diagnostic: every OnClientSayCommand invocation is logged at Info level
 * with teamOnly / isCommand / commandName / message so we can see exactly
 * what the chat parser delivers.
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
using Sharp.Shared.Units;

namespace Cs2Surf.MapCommand;

public sealed class SurfMapCommand : IModSharpModule, IClientListener
{
    public string DisplayName   => "Cs2Surf.MapCommand";
    public string DisplayAuthor => "hwcopeland";

    private readonly ISharedSystem           _shared;
    private readonly ILogger<SurfMapCommand> _logger;
    private readonly IClientManager          _clientManager;
    private readonly string                  _sharpPath;

    // RTV state. Resets every map change (we detect via a dead-simple
    // "cleared on first say after N seconds of silence" approach — if it
    // becomes a problem, wire up a game listener later).
    private readonly HashSet<SteamID> _rtvVoters      = [];
    private          string?         _lastNomination;

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
        _clientManager.InstallClientListener(this);
        _logger.LogInformation(
            "SurfMapCommand loaded — !map (admin), !rtv, !nominate available");
    }

    public void Shutdown()
    {
        _clientManager.RemoveClientListener(this);
    }

    int IClientListener.ListenerVersion  => IClientListener.ApiVersion;
    int IClientListener.ListenerPriority => 0;

    public ECommandAction OnClientSayCommand(IGameClient client,
                                             bool        teamOnly,
                                             bool        isCommand,
                                             string      commandName,
                                             string      message)
    {
        // Brute-force diagnostic so we can see exactly what the parser delivers
        // for every chat line. Strip once we confirm !map / !rtv routing works.
        _logger.LogInformation(
            "SAY from {SteamId} team={Team} isCmd={IsCmd} cmd={Cmd!r} msg={Msg!r}",
            client.SteamId, teamOnly, isCommand, commandName, message);

        if (!isCommand)
        {
            return ECommandAction.Skipped;
        }

        var cmd = (commandName ?? string.Empty).ToLowerInvariant();
        var arg = (message    ?? string.Empty).Trim();

        // Some parsers include the command word in `message` too; strip it.
        if (arg.StartsWith("!" + cmd, StringComparison.OrdinalIgnoreCase)
            || arg.StartsWith("." + cmd, StringComparison.OrdinalIgnoreCase))
        {
            arg = arg.Substring(cmd.Length + 1).Trim();
        }

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

            default:
                return ECommandAction.Skipped;
        }
    }

    // --- !map <id_or_name> --------------------------------------------------

    private ECommandAction HandleMap(IGameClient client, string arg)
    {
        var admin = _clientManager.FindAdmin(client.SteamId);
        if (admin is null || !admin.HasPermission("admin:map"))
        {
            _logger.LogInformation(
                "!map denied for {SteamId}: no admin:map permission", client.SteamId);
            return ECommandAction.Handled;
        }

        if (string.IsNullOrWhiteSpace(arg))
        {
            _logger.LogInformation("!map from {SteamId}: no arg", client.SteamId);
            return ECommandAction.Handled;
        }

        var modSharp = _shared.GetModSharp();
        if (long.TryParse(arg, out _))
        {
            _logger.LogInformation(
                "Admin {SteamId} → host_workshop_map {WorkshopId}", client.SteamId, arg);
            modSharp.ServerCommand($"host_workshop_map {arg}");
        }
        else
        {
            _logger.LogInformation(
                "Admin {SteamId} → changelevel {Map}", client.SteamId, arg);
            modSharp.ServerCommand($"changelevel {arg}");
        }

        _rtvVoters.Clear();
        return ECommandAction.Handled;
    }

    // --- !rtv ---------------------------------------------------------------
    //
    // Counts distinct voters by SteamID. Threshold: floor(connected/2)+1.
    // When reached, rotate to the next workshop ID from maprotation.txt.

    private ECommandAction HandleRtv(IGameClient client)
    {
        _rtvVoters.Add(client.SteamId);

        var connected = CountConnectedPlayers();
        var needed    = Math.Max(1, (connected / 2) + 1);
        var have      = _rtvVoters.Count;

        _logger.LogInformation(
            "!rtv {SteamId}: {Have}/{Needed} (connected={Conn})",
            client.SteamId, have, needed, connected);

        if (have < needed)
        {
            return ECommandAction.Handled;
        }

        _logger.LogInformation("RTV threshold reached — rotating");

        var nextId = PickNextMap();
        if (nextId is null)
        {
            _logger.LogWarning("RTV: no maprotation.txt entries — skipping");
            return ECommandAction.Handled;
        }

        _shared.GetModSharp().ServerCommand($"host_workshop_map {nextId}");
        _rtvVoters.Clear();
        _lastNomination = null;
        return ECommandAction.Handled;
    }

    // --- !nominate <id> -----------------------------------------------------

    private ECommandAction HandleNominate(IGameClient client, string arg)
    {
        if (string.IsNullOrWhiteSpace(arg) || !long.TryParse(arg, out _))
        {
            _logger.LogInformation(
                "!nominate from {SteamId}: invalid arg {Arg!r}", client.SteamId, arg);
            return ECommandAction.Handled;
        }

        _lastNomination = arg.Trim();
        _logger.LogInformation(
            "!nominate {SteamId} → {Map}", client.SteamId, _lastNomination);
        return ECommandAction.Handled;
    }

    // --- helpers ------------------------------------------------------------

    private int CountConnectedPlayers()
    {
        var count = 0;
        // Iterate client slots 0..63 (max players); fake clients excluded.
        for (var slot = 0; slot < 64; slot++)
        {
            var c = _clientManager.GetGameClient(slot);
            if (c is not null && !c.IsFakeClient && c.IsAuthenticated)
            {
                count++;
            }
        }
        return count;
    }

    private string? PickNextMap()
    {
        // Nomination wins if set.
        if (!string.IsNullOrWhiteSpace(_lastNomination))
        {
            return _lastNomination;
        }

        // Otherwise read maprotation.txt and pick the first non-comment line.
        // MVP: always picks the first entry. Good enough to prove the loop.
        var path = Path.Combine(_sharpPath, "configs", "maprotation.txt");
        if (!File.Exists(path))
        {
            return null;
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
                return line;
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to read maprotation.txt");
        }

        return null;
    }
}
