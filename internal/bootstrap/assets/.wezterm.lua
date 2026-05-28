local wezterm = require("wezterm")
local config = wezterm.config_builder()

if wezterm.target_triple == "x86_64-pc-windows-msvc" then
	-- Since WezTerm is now elevated, pwsh will automatically launch as admin inside it
	config.default_prog = { "pwsh.exe", "-NoLogo" }
end
config.keys = {
	-- Example - CTRL+SHIFT+U to open your WSL Ubuntu distribution
	{
		key = "U",
		mods = "CTRL|SHIFT",
		action = wezterm.action.SpawnCommandInNewTab({
			domain = { DomainName = "WSL:Ubuntu-24.04" },
		}),
	},

	{
		key = "R",
		mods = "CTRL|SHIFT",
		action = wezterm.action.SpawnCommandInNewTab({
			domain = { DomainName = "WSL:fedoraremix" },
		}),
	},
}
return config
