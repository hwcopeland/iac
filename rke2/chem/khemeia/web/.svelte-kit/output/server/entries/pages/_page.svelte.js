import "../../chunks/index-server.js";
import { L as attr, R as escape_html, a as ensure_array_like, i as derived, n as attr_class, r as bind_props, s as stringify } from "../../chunks/dev.js";
//#region src/lib/viewer.ts
var plugin = null;
async function loadPdb(id) {
	if (!plugin) throw new Error("Viewer not initialized");
	plugin.clear();
	const url = `https://files.rcsb.org/download/${id.toUpperCase()}.cif`;
	await plugin.loadStructureFromUrl(url, "mmcif", false);
}
function resetCamera() {
	try {
		plugin?.managers?.camera?.reset?.();
	} catch {}
}
function toggleSpin(enabled) {
	try {
		if (plugin?.canvas3d) plugin.canvas3d.setProps({ trackball: {
			...plugin.canvas3d.props.trackball,
			spin: enabled
		} });
	} catch {}
}
//#endregion
//#region src/lib/components/Toolbar.svelte
function Toolbar($$renderer, $$props) {
	$$renderer.component(($$renderer) => {
		let { activeTab = "explorer", onCommandPalette } = $$props;
		const tabs = [
			{
				id: "explorer",
				label: "Explorer"
			},
			{
				id: "builder",
				label: "Builder"
			},
			{
				id: "calculations",
				label: "Calculations"
			}
		];
		const modKey = typeof navigator !== "undefined" && navigator.platform?.includes("Mac") ? "⌘" : "Ctrl";
		$$renderer.push(`<header class="toolbar svelte-1ld6r3r"><div class="toolbar-left svelte-1ld6r3r"><span class="logo svelte-1ld6r3r">khemeia</span></div> <nav class="toolbar-tabs svelte-1ld6r3r"><!--[-->`);
		const each_array = ensure_array_like(tabs);
		for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
			let tab = each_array[$$index];
			$$renderer.push(`<button${attr_class("tab-btn svelte-1ld6r3r", void 0, { "active": activeTab === tab.id })}>${escape_html(tab.label)}</button>`);
		}
		$$renderer.push(`<!--]--></nav> <div class="toolbar-right svelte-1ld6r3r"><button class="cmd-hint svelte-1ld6r3r">${escape_html(modKey)}+K</button></div></header>`);
		bind_props($$props, { activeTab });
	});
}
//#endregion
//#region src/lib/components/Panel.svelte
function Panel($$renderer, $$props) {
	$$renderer.component(($$renderer) => {
		let { title, collapsed = false, children } = $$props;
		$$renderer.push(`<section${attr_class("panel svelte-hxsa5u", void 0, { "collapsed": collapsed })}><button class="panel-header svelte-hxsa5u"><span class="panel-title">${escape_html(title)}</span> <span class="panel-chevron svelte-hxsa5u">${escape_html(collapsed ? "▶" : "▼")}</span></button> `);
		if (!collapsed) {
			$$renderer.push("<!--[0-->");
			$$renderer.push(`<div class="panel-body svelte-hxsa5u">`);
			children($$renderer);
			$$renderer.push(`<!----></div>`);
		} else $$renderer.push("<!--[-1-->");
		$$renderer.push(`<!--]--></section>`);
		bind_props($$props, { collapsed });
	});
}
//#endregion
//#region src/lib/components/ExplorerPanel.svelte
function ExplorerPanel($$renderer, $$props) {
	$$renderer.component(($$renderer) => {
		let pdbId = "";
		let spinning = false;
		const representations = [
			"Cartoon",
			"Ball & Stick",
			"Spacefill",
			"Wireframe"
		];
		const colorSchemes = [
			"Element",
			"Chain",
			"Secondary Structure",
			"Hydrophobicity"
		];
		$$renderer.push(`<div class="explorer-panels svelte-vcd5nr">`);
		Panel($$renderer, {
			title: "Load Structure",
			children: ($$renderer) => {
				$$renderer.push(`<div class="input-row svelte-vcd5nr"><input type="text" class="text-input svelte-vcd5nr" placeholder="PDB ID (e.g. 1crn)"${attr("value", pdbId)}/> <button class="btn btn-accent svelte-vcd5nr"${attr("disabled", !pdbId.trim(), true)}>${escape_html("Load")}</button></div> <button class="link-btn svelte-vcd5nr">Upload file (.pdb, .cif, .mol, .sdf, .xyz)</button> <input type="file" accept=".pdb,.cif,.mmcif,.mol,.mol2,.sdf,.xyz" style="display: none"/> `);
				$$renderer.push("<!--[-1-->");
				$$renderer.push(`<!--]-->`);
			},
			$$slots: { default: true }
		});
		$$renderer.push(`<!----> `);
		Panel($$renderer, {
			title: "Representation",
			children: ($$renderer) => {
				$$renderer.push(`<div class="btn-grid svelte-vcd5nr"><!--[-->`);
				const each_array = ensure_array_like(representations);
				for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
					let rep = each_array[$$index];
					$$renderer.push(`<button class="btn btn-small svelte-vcd5nr">${escape_html(rep)}</button>`);
				}
				$$renderer.push(`<!--]--></div>`);
			},
			$$slots: { default: true }
		});
		$$renderer.push(`<!----> `);
		Panel($$renderer, {
			title: "Color Scheme",
			children: ($$renderer) => {
				$$renderer.push(`<div class="btn-grid svelte-vcd5nr"><!--[-->`);
				const each_array_1 = ensure_array_like(colorSchemes);
				for (let $$index_1 = 0, $$length = each_array_1.length; $$index_1 < $$length; $$index_1++) {
					let scheme = each_array_1[$$index_1];
					$$renderer.push(`<button class="btn btn-small svelte-vcd5nr">${escape_html(scheme)}</button>`);
				}
				$$renderer.push(`<!--]--></div>`);
			},
			$$slots: { default: true }
		});
		$$renderer.push(`<!----> `);
		Panel($$renderer, {
			title: "Controls",
			children: ($$renderer) => {
				$$renderer.push(`<div class="btn-row svelte-vcd5nr"><button class="btn btn-small svelte-vcd5nr">Reset View</button> <button${attr_class("btn btn-small svelte-vcd5nr", void 0, { "active": spinning })}>${escape_html("Spin")}</button></div>`);
			},
			$$slots: { default: true }
		});
		$$renderer.push(`<!----></div>`);
	});
}
//#endregion
//#region src/lib/components/BuilderPanel.svelte
function BuilderPanel($$renderer) {
	let element = "";
	let smiles = "";
	const tools = [
		{
			id: "place",
			label: "Place"
		},
		{
			id: "bond",
			label: "Bond"
		},
		{
			id: "delete",
			label: "Delete"
		}
	];
	let activeTool = null;
	$$renderer.push(`<div class="builder-panels svelte-99bk1h">`);
	Panel($$renderer, {
		title: "Element",
		children: ($$renderer) => {
			$$renderer.push(`<input type="text" class="text-input full svelte-99bk1h" placeholder="Element symbol (e.g. C, N, O)"${attr("value", element)}/>`);
		},
		$$slots: { default: true }
	});
	$$renderer.push(`<!----> `);
	Panel($$renderer, {
		title: "SMILES",
		children: ($$renderer) => {
			$$renderer.push(`<div class="input-row svelte-99bk1h"><input type="text" class="text-input svelte-99bk1h" placeholder="e.g. CCO"${attr("value", smiles)}/> <button class="btn btn-accent svelte-99bk1h"${attr("disabled", !smiles.trim(), true)}>Build</button></div>`);
		},
		$$slots: { default: true }
	});
	$$renderer.push(`<!----> `);
	Panel($$renderer, {
		title: "Tools",
		children: ($$renderer) => {
			$$renderer.push(`<div class="btn-row svelte-99bk1h"><!--[-->`);
			const each_array = ensure_array_like(tools);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let tool = each_array[$$index];
				$$renderer.push(`<button${attr_class("btn btn-small svelte-99bk1h", void 0, { "active": activeTool === tool.id })}>${escape_html(tool.label)}</button>`);
			}
			$$renderer.push(`<!--]--></div> <p class="hint svelte-99bk1h">Canvas editing coming in v0.2</p>`);
		},
		$$slots: { default: true }
	});
	$$renderer.push(`<!----></div>`);
}
//#endregion
//#region src/lib/components/CalculationsPanel.svelte
function CalculationsPanel($$renderer, $$props) {
	$$renderer.component(($$renderer) => {
		$$renderer.push(`<div class="calc-panels svelte-1gxnl78">`);
		$$renderer.push("<!--[0-->");
		$$renderer.push(`<div class="loading svelte-1gxnl78">Loading plugins...</div>`);
		$$renderer.push(`<!--]--></div>`);
	});
}
//#endregion
//#region src/lib/components/StatusBar.svelte
function StatusBar($$renderer, $$props) {
	$$renderer.component(($$renderer) => {
		let { hoverInfo } = $$props;
		$$renderer.push(`<footer class="status-bar svelte-1piydef">`);
		if (hoverInfo) {
			$$renderer.push("<!--[0-->");
			$$renderer.push(`<span class="status-element svelte-1piydef">${escape_html(hoverInfo.element)}</span> <span class="status-atom svelte-1piydef">${escape_html(hoverInfo.atomName)}</span> <span class="status-sep svelte-1piydef">·</span> <span class="status-residue svelte-1piydef">${escape_html(hoverInfo.residueName)} ${escape_html(hoverInfo.residueId)}</span> <span class="status-sep svelte-1piydef">·</span> <span class="status-chain svelte-1piydef">Chain ${escape_html(hoverInfo.chainId)}</span> <span class="status-sep svelte-1piydef">·</span> <span class="status-coords svelte-1piydef">(${escape_html(hoverInfo.x.toFixed(1))}, ${escape_html(hoverInfo.y.toFixed(1))}, ${escape_html(hoverInfo.z.toFixed(1))})</span>`);
		} else {
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<span class="status-idle svelte-1piydef">Hover over atoms to see details</span>`);
		}
		$$renderer.push(`<!--]--></footer>`);
	});
}
//#endregion
//#region src/lib/components/CommandPalette.svelte
function CommandPalette($$renderer, $$props) {
	$$renderer.component(($$renderer) => {
		let { open = false } = $$props;
		let query = "";
		let selectedIndex = 0;
		const actions = [
			{
				id: "load-1crn",
				label: "Load Crambin (1CRN)",
				hint: "demo structure",
				handler: () => {
					loadPdb("1crn");
					close();
				}
			},
			{
				id: "load-4hhb",
				label: "Load Hemoglobin (4HHB)",
				hint: "demo structure",
				handler: () => {
					loadPdb("4hhb");
					close();
				}
			},
			{
				id: "load-1ubq",
				label: "Load Ubiquitin (1UBQ)",
				hint: "demo structure",
				handler: () => {
					loadPdb("1ubq");
					close();
				}
			},
			{
				id: "reset",
				label: "Reset Camera",
				hint: "center view",
				handler: () => {
					resetCamera();
					close();
				}
			},
			{
				id: "spin",
				label: "Toggle Spin",
				hint: "rotate",
				handler: () => {
					toggleSpin(true);
					close();
				}
			}
		];
		let filtered = derived(() => (query.trim(), actions));
		function close() {
			open = false;
		}
		if (open) {
			$$renderer.push("<!--[0-->");
			$$renderer.push(`<div class="palette-backdrop svelte-wh9uu8"><div class="palette svelte-wh9uu8" role="dialog" aria-label="Command palette"><input type="text" class="palette-input svelte-wh9uu8" placeholder="Type a command..."${attr("value", query)}/> <ul class="palette-list svelte-wh9uu8"><!--[-->`);
			const each_array = ensure_array_like(filtered());
			for (let i = 0, $$length = each_array.length; i < $$length; i++) {
				let action = each_array[i];
				$$renderer.push(`<li><button${attr_class("palette-item svelte-wh9uu8", void 0, { "selected": i === selectedIndex })}><span class="palette-label svelte-wh9uu8">${escape_html(action.label)}</span> `);
				if (action.hint) {
					$$renderer.push("<!--[0-->");
					$$renderer.push(`<span class="palette-hint svelte-wh9uu8">${escape_html(action.hint)}</span>`);
				} else $$renderer.push("<!--[-1-->");
				$$renderer.push(`<!--]--></button></li>`);
			}
			$$renderer.push(`<!--]--> `);
			if (filtered().length === 0) {
				$$renderer.push("<!--[0-->");
				$$renderer.push(`<li class="palette-empty svelte-wh9uu8">No matching commands</li>`);
			} else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></ul></div></div>`);
		} else $$renderer.push("<!--[-1-->");
		$$renderer.push(`<!--]-->`);
		bind_props($$props, { open });
	});
}
//#endregion
//#region src/lib/components/Toast.svelte
var toasts = [];
function Toast($$renderer) {
	if (toasts.length > 0) {
		$$renderer.push("<!--[0-->");
		$$renderer.push(`<div class="toast-container svelte-1cpok13"><!--[-->`);
		const each_array = ensure_array_like(toasts);
		for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
			let t = each_array[$$index];
			$$renderer.push(`<div${attr_class(`toast toast-${stringify(t.type)}`, "svelte-1cpok13")}><span class="toast-msg svelte-1cpok13">${escape_html(t.message)}</span> <button class="toast-close svelte-1cpok13">x</button></div>`);
		}
		$$renderer.push(`<!--]--></div>`);
	} else $$renderer.push("<!--[-1-->");
	$$renderer.push(`<!--]-->`);
}
//#endregion
//#region src/routes/+page.svelte
function _page($$renderer, $$props) {
	$$renderer.component(($$renderer) => {
		let activeTab = "explorer";
		let hoverInfo = null;
		let commandPaletteOpen = false;
		let $$settled = true;
		let $$inner_renderer;
		function $$render_inner($$renderer) {
			$$renderer.push(`<div class="app svelte-1uha8ag">`);
			Toolbar($$renderer, {
				onCommandPalette: () => commandPaletteOpen = true,
				get activeTab() {
					return activeTab;
				},
				set activeTab($$value) {
					activeTab = $$value;
					$$settled = false;
				}
			});
			$$renderer.push(`<!----> <div class="main svelte-1uha8ag"><div class="viewer-area svelte-1uha8ag"><div class="viewer-container svelte-1uha8ag"></div> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></div> `);
			$$renderer.push("<!--[0-->");
			$$renderer.push(`<aside class="side-panel svelte-1uha8ag"><div class="side-panel-scroll svelte-1uha8ag">`);
			if (activeTab === "explorer") {
				$$renderer.push("<!--[0-->");
				ExplorerPanel($$renderer, {});
			} else if (activeTab === "builder") {
				$$renderer.push("<!--[1-->");
				BuilderPanel($$renderer, {});
			} else if (activeTab === "calculations") {
				$$renderer.push("<!--[2-->");
				CalculationsPanel($$renderer, {});
			} else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></div></aside>`);
			$$renderer.push(`<!--]--></div> `);
			StatusBar($$renderer, { hoverInfo });
			$$renderer.push(`<!----> `);
			CommandPalette($$renderer, {
				get open() {
					return commandPaletteOpen;
				},
				set open($$value) {
					commandPaletteOpen = $$value;
					$$settled = false;
				}
			});
			$$renderer.push(`<!----> `);
			Toast($$renderer, {});
			$$renderer.push(`<!----></div>`);
		}
		do {
			$$settled = true;
			$$inner_renderer = $$renderer.copy();
			$$render_inner($$inner_renderer);
		} while (!$$settled);
		$$renderer.subsume($$inner_renderer);
	});
}
//#endregion
export { _page as default };
