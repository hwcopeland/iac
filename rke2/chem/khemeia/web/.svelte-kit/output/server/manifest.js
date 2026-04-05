export const manifest = (() => {
function __memo(fn) {
	let value;
	return () => value ??= (value = fn());
}

return {
	appDir: "_app",
	appPath: "_app",
	assets: new Set(["molstar.css","molstar.js"]),
	mimeTypes: {".css":"text/css",".js":"text/javascript"},
	_: {
		client: {start:"_app/immutable/entry/start.COJ7XLSe.js",app:"_app/immutable/entry/app.Ja35AYTZ.js",imports:["_app/immutable/entry/start.COJ7XLSe.js","_app/immutable/chunks/BMZZoNiX.js","_app/immutable/chunks/CAx8lw1w.js","_app/immutable/entry/app.Ja35AYTZ.js","_app/immutable/chunks/CAx8lw1w.js","_app/immutable/chunks/Dj6f-nJM.js","_app/immutable/chunks/DEDqjojZ.js"],stylesheets:[],fonts:[],uses_env_dynamic_public:false},
		nodes: [
			__memo(() => import('./nodes/0.js')),
			__memo(() => import('./nodes/1.js'))
		],
		remotes: {
			
		},
		routes: [
			
		],
		prerendered_routes: new Set(["/"]),
		matchers: async () => {
			
			return {  };
		},
		server_assets: {}
	}
}
})();
