-- Neovim headless test: verify LSP client does NOT attach to a non-matching filetype.
-- Env vars: LUX_CMD, LUX_FILE
-- Exit codes: 0 = correctly not attached, 1 = unexpectedly attached
--
-- Usage: LUX_CMD="/path/to/lux lsp" LUX_FILE=test.txt \
--        nvim --headless --clean -c "luafile check_no_attach.lua"

local lux_cmd_str = vim.env.LUX_CMD
local test_file = vim.env.LUX_FILE

if not lux_cmd_str or not test_file then
  io.stderr:write("ERROR: set LUX_CMD, LUX_FILE env vars\n")
  vim.cmd("cquit 2")
  return
end

local cmd = {}
for word in lux_cmd_str:gmatch("%S+") do
  table.insert(cmd, word)
end

vim.lsp.config("lux", {
  cmd = cmd,
  root_markers = { "go.mod", ".git" },
  filetypes = { "go" },
})
vim.lsp.enable("lux")

vim.cmd("edit " .. vim.fn.fnameescape(test_file))

-- Wait briefly — client should NOT attach
local attached = vim.wait(5000, function()
  local clients = vim.lsp.get_clients({ name = "lux", bufnr = 0 })
  return #clients > 0
end, 500)

if attached then
  io.stderr:write("UNEXPECTEDLY_ATTACHED\n")
  vim.cmd("cquit 1")
  return
end

io.stderr:write("CORRECTLY_NOT_ATTACHED\n")
vim.cmd("quit")
