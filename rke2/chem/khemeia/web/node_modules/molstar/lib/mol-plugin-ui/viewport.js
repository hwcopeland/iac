import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { throttleTime } from 'rxjs';
import { PluginCommands } from '../mol-plugin/commands.js';
import { PluginConfig } from '../mol-plugin/config.js';
import { PluginUIComponent } from './base.js';
import { Button, ControlGroup, IconButton } from './controls/common.js';
import { AspectRatioSvg, AutorenewSvg, BuildOutlinedSvg, CameraOutlinedSvg, CloseSvg, FullscreenSvg, HeadsetVRSvg, LightModeSvg, TuneSvg } from './controls/icons.js';
import { ToggleSelectionModeButton } from './structure/selection.js';
import { ViewportCanvas } from './viewport/canvas.js';
import { DownloadScreenshotControls } from './viewport/screenshot.js';
import { SimpleSettingsControl } from './viewport/simple-settings.js';
export class ViewportControls extends PluginUIComponent {
    constructor() {
        super(...arguments);
        this.allCollapsedState = {
            isSettingsExpanded: false,
            isScreenshotExpanded: false,
        };
        this.state = {
            ...this.allCollapsedState,
            isCameraResetEnabled: true,
        };
        this.resetCamera = () => {
            PluginCommands.Camera.Reset(this.plugin, {});
        };
        this.toggleSettingsExpanded = this.toggle('isSettingsExpanded');
        this.toggleScreenshotExpanded = this.toggle('isScreenshotExpanded');
        this.toggleControls = () => {
            PluginCommands.Layout.Update(this.plugin, { state: { showControls: !this.plugin.layout.state.showControls } });
        };
        this.toggleExpanded = () => {
            PluginCommands.Layout.Update(this.plugin, {
                state: {
                    isExpanded: !this.plugin.layout.state.isExpanded,
                    expandToFullscreen: false
                }
            });
        };
        this.toggleFullscreen = () => {
            PluginCommands.Layout.Update(this.plugin, {
                state: {
                    expandToFullscreen: !this.plugin.layout.state.expandToFullscreen,
                }
            });
        };
        this.toggleXR = () => {
            if (this.plugin.canvas3d) {
                if (this.plugin.canvas3d.xr.isPresenting.value) {
                    this.plugin.canvas3d.xr.end();
                }
                else {
                    this.plugin.canvas3d.xr.request();
                }
            }
        };
        this.toggleIllumination = () => {
            if (!this.plugin.canvas3d)
                return;
            PluginCommands.Canvas3D.SetSettings(this.plugin, {
                settings: {
                    illumination: {
                        ...this.plugin.canvas3d.props.illumination,
                        enabled: !this.plugin.canvas3d.props.illumination.enabled
                    }
                }
            });
        };
        this.setSettings = (p) => {
            PluginCommands.Canvas3D.SetSettings(this.plugin, { settings: { [p.name]: p.value } });
        };
        this.setLayout = (p) => {
            PluginCommands.Layout.Update(this.plugin, { state: { [p.name]: p.value } });
        };
        this.screenshot = () => {
            var _a;
            (_a = this.plugin.helpers.viewportScreenshot) === null || _a === void 0 ? void 0 : _a.download();
        };
        this.enableCameraReset = (enable) => {
            this.setState(old => ({ ...old, isCameraResetEnabled: enable }));
        };
    }
    toggle(panel) {
        return (e) => {
            this.setState(old => ({ ...old, ...this.allCollapsedState, [panel]: !this.state[panel] }));
            e === null || e === void 0 ? void 0 : e.currentTarget.blur();
        };
    }
    componentDidMount() {
        this.subscribe(this.plugin.events.canvas3d.settingsUpdated, () => this.forceUpdate());
        this.subscribe(this.plugin.layout.events.updated, () => this.forceUpdate());
        if (this.plugin.canvas3d) {
            this.subscribe(this.plugin.canvas3d.camera.stateChanged.pipe(throttleTime(500, undefined, { leading: true, trailing: true })), snapshot => this.enableCameraReset(snapshot.radius !== 0 && snapshot.radiusMax !== 0));
            this.subscribe(this.plugin.canvas3d.xr.isSupported, () => this.forceUpdate());
            this.subscribe(this.plugin.canvas3d.xr.isPresenting, () => this.forceUpdate());
        }
    }
    icon(icon, onClick, title, isOn = true, disabled = false) {
        return _jsx(IconButton, { svg: icon, toggleState: isOn, onClick: onClick, title: title, style: { background: 'transparent' }, disabled: disabled });
    }
    render() {
        var _a, _b, _c;
        const showXr = this.plugin.config.get(PluginConfig.Viewport.ShowXR);
        const xrIsSupported = !!((_a = this.plugin.canvas3d) === null || _a === void 0 ? void 0 : _a.xr.isSupported.value);
        const xrIsPresenting = !!((_b = this.plugin.canvas3d) === null || _b === void 0 ? void 0 : _b.xr.isPresenting.value);
        const xr = showXr === 'always' || (showXr === 'auto' && xrIsSupported);
        const xrTitle = !xrIsSupported ? 'Augmented/Virtual Reality unavailable' : (xrIsPresenting ? 'Exit XR' : 'Enter XR');
        const layoutState = this.plugin.layout.state;
        return _jsxs("div", { className: 'msp-viewport-controls', children: [_jsxs("div", { className: 'msp-viewport-controls-buttons', children: [this.plugin.config.get(PluginConfig.Viewport.ShowReset) &&
                            _jsxs("div", { className: 'msp-hover-box-wrapper', children: [_jsx("div", { className: 'msp-semi-transparent-background' }), this.icon(AutorenewSvg, this.resetCamera, 'Reset Zoom'), _jsx("div", { className: 'msp-hover-box-body', children: _jsxs("div", { className: 'msp-flex-column', children: [_jsx("div", { className: 'msp-flex-row', children: _jsx(Button, { onClick: () => this.resetCamera(), disabled: !this.state.isCameraResetEnabled, title: 'Set camera zoom to fit the visible scene into view', children: "Reset Zoom" }) }), _jsx("div", { className: 'msp-flex-row', children: _jsx(Button, { onClick: () => PluginCommands.Camera.OrientAxes(this.plugin), disabled: !this.state.isCameraResetEnabled, title: 'Align principal component axes of the loaded structures to the screen axes (\u201Clay flat\u201D)', children: "Orient Axes" }) }), _jsx("div", { className: 'msp-flex-row', children: _jsx(Button, { onClick: () => PluginCommands.Camera.ResetAxes(this.plugin), disabled: !this.state.isCameraResetEnabled, title: 'Align Cartesian axes to the screen axes', children: "Reset Axes" }) })] }) }), _jsx("div", { className: 'msp-hover-box-spacer' })] }), this.plugin.config.get(PluginConfig.Viewport.ShowScreenshotControls) && _jsxs("div", { children: [_jsx("div", { className: 'msp-semi-transparent-background' }), this.icon(CameraOutlinedSvg, this.toggleScreenshotExpanded, 'Screenshot / State Snapshot', this.state.isScreenshotExpanded)] }), _jsxs("div", { children: [_jsx("div", { className: 'msp-semi-transparent-background' }), this.plugin.config.get(PluginConfig.Viewport.ShowControls) && this.icon(BuildOutlinedSvg, this.toggleControls, 'Toggle Controls Panel', this.plugin.layout.state.showControls), this.plugin.config.get(PluginConfig.Viewport.ShowExpand) && _jsxs("div", { className: 'msp-hover-box-wrapper', children: [this.icon(FullscreenSvg, this.toggleExpanded, 'Toggle Expanded Viewport', this.plugin.layout.state.isExpanded), _jsx("div", { className: 'msp-hover-box-body', children: _jsx("div", { className: 'msp-flex-column', children: _jsx("div", { className: 'msp-flex-row', children: _jsx(Button, { onClick: this.toggleFullscreen, children: layoutState.expandToFullscreen ? 'Exit Fullscreen' : 'Fullscreen' }) }) }) }), _jsx("div", { className: 'msp-hover-box-spacer' })] }), !this.plugin.config.get(PluginConfig.Viewport.ShowExpand)
                                    && this.plugin.config.get(PluginConfig.Viewport.ShowToggleFullscreen)
                                    && this.icon(AspectRatioSvg, this.toggleFullscreen, 'Toggle Full Screen', this.plugin.layout.state.expandToFullscreen), this.plugin.config.get(PluginConfig.Viewport.ShowSettings) && this.icon(TuneSvg, this.toggleSettingsExpanded, 'Settings / Controls Info', this.state.isSettingsExpanded), this.plugin.config.get(PluginConfig.Viewport.ShowIllumination) && this.icon(LightModeSvg, this.toggleIllumination, 'Illumination', ((_c = this.plugin.canvas3d) === null || _c === void 0 ? void 0 : _c.props.illumination.enabled) || false), xr && this.icon(HeadsetVRSvg, this.toggleXR, xrTitle, xrIsPresenting, !xrIsSupported)] }), this.plugin.config.get(PluginConfig.Viewport.ShowSelectionMode) && _jsxs("div", { children: [_jsx("div", { className: 'msp-semi-transparent-background' }), _jsx(ToggleSelectionModeButton, {})] })] }), this.state.isScreenshotExpanded && _jsx("div", { className: 'msp-viewport-controls-panel', children: _jsx(ControlGroup, { header: 'Screenshot / State', title: 'Click to close.', initialExpanded: true, hideExpander: true, hideOffset: true, onHeaderClick: this.toggleScreenshotExpanded, topRightIcon: CloseSvg, noTopMargin: true, childrenClassName: 'msp-viewport-controls-panel-controls', children: _jsx(DownloadScreenshotControls, { close: this.toggleScreenshotExpanded }) }) }), this.state.isSettingsExpanded && _jsx("div", { className: 'msp-viewport-controls-panel', children: _jsx(ControlGroup, { header: 'Settings / Controls Info', title: 'Click to close.', initialExpanded: true, hideExpander: true, hideOffset: true, onHeaderClick: this.toggleSettingsExpanded, topRightIcon: CloseSvg, noTopMargin: true, childrenClassName: 'msp-viewport-controls-panel-controls', children: _jsx(SimpleSettingsControl, {}) }) })] });
    }
}
export const Logo = () => _jsx("a", { className: 'msp-logo', href: 'https://molstar.org', target: '_blank' });
export const Viewport = () => _jsx(ViewportCanvas, { logo: Logo });
