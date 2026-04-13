/*
 * SurfMapCommand — minimum viable ModSharp module providing !map in chat.
 *
 * Hooks IClientListener.OnClientSayCommand and, when an admin types "!map
 * <workshop_id_or_name>", issues the appropriate server command to change
 * map. Uses the server's native AdminManager for permission checks.
 *
 * Intentionally tiny: no TnmsPluginFoundation, no CounterStrikeSharp, no
 * extra deps. The entire feature is ~80 lines because the underlying
 * engine/admin APIs already do everything — we just need a command sink
 * that reliably runs on this server without getting swallowed by Tnms or
 * other third-party chat listeners.
 */

using System;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging;
using Sharp.Shared;
using Sharp.Shared.Enums;
using Sharp.Shared.Listeners;
using Sharp.Shared.Managers;
using Sharp.Shared.Objects;

namespace Cs2Surf.MapCommand;

public sealed class SurfMapCommand : IModSharpModule, IClientListener
{
    public string DisplayName   => "Cs2Surf.MapCommand";
    public string DisplayAuthor => "hwcopeland";

    private readonly ISharedSystem           _shared;
    private readonly ILogger<SurfMapCommand> _logger;
    private readonly IClientManager          _clientManager;

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
    }

    public bool Init() => true;

    public void PostInit()
    {
        _clientManager.InstallClientListener(this);
        _logger.LogInformation("SurfMapCommand loaded — !map <id_or_name> for admins with admin:map");
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
        // Only handle !map / .map triggers. Any other chat passes through.
        if (!isCommand || !string.Equals(commandName, "map", StringComparison.OrdinalIgnoreCase))
        {
            return ECommandAction.Skipped;
        }

        // Admin check via native AdminManager cache.
        var admin = _clientManager.FindAdmin(client.SteamId);
        if (admin is null || !admin.HasPermission("admin:map"))
        {
            _logger.LogDebug("!map denied for {SteamId}: no admin:map permission", client.SteamId);
            return ECommandAction.Handled;
        }

        // message is everything after "!map " — the rest of the line.
        // commandName already stripped the "map" word.
        var arg = message?.Trim() ?? string.Empty;

        // ModSharp sometimes passes the full "!map <arg>" in message depending
        // on the chat parser. Handle both cases by stripping a leading "!map"
        // or ".map" if present.
        if (arg.StartsWith("!map", StringComparison.OrdinalIgnoreCase)
            || arg.StartsWith(".map", StringComparison.OrdinalIgnoreCase))
        {
            arg = arg.Substring(4).Trim();
        }

        if (string.IsNullOrWhiteSpace(arg))
        {
            _logger.LogInformation("!map from {SteamId}: no argument — showing usage", client.SteamId);
            return ECommandAction.Handled;
        }

        // Numeric → workshop ID → host_workshop_map
        // Otherwise → map name → changelevel (works for mounted/built-in maps)
        var modSharp = _shared.GetModSharp();
        if (long.TryParse(arg, out _))
        {
            _logger.LogInformation("Admin {SteamId} → host_workshop_map {WorkshopId}", client.SteamId, arg);
            modSharp.ServerCommand($"host_workshop_map {arg}");
        }
        else
        {
            _logger.LogInformation("Admin {SteamId} → changelevel {Map}", client.SteamId, arg);
            modSharp.ServerCommand($"changelevel {arg}");
        }

        return ECommandAction.Handled;
    }
}
