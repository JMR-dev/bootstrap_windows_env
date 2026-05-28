local wezterm = require 'wezterm'

return {
  color_scheme = 'Builtin Solarized Dark',
  font = wezterm.font('JetBrainsMono Nerd Font'),
  font_size = 11.0,
  default_prog = { 'pwsh.exe', '-NoLogo' },
  window_decorations = 'RESIZE',
}
