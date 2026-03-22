-- Neovim headless test: verify LSP client attachment and capabilities.
-- Env vars: LUX_CMD, LUX_FILE
-- Exit codes: 0 = attached with formatting, 1 = error, 2 = not attached
--
-- Usage: LUX_CMD="/path/to/lux lsp" LUX_FILE=test.go \
--        nvim --headless --clean -c "luafile check_attach.lua"

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

-- Wait for LSP to attach
local attached = vim.wait(30000, function()
  local clients = vim.lsp.get_clients({ name = "lux", bufnr = 0 })
  return #clients > 0
end, 500)

if not attached then
  io.stderr:write("NOT_ATTACHED\n")
  vim.cmd("cquit 2")
  return
end

-- Check formatting capability
local has_format = vim.wait(10000, function()
  local clients = vim.lsp.get_clients({ name = "lux", bufnr = 0 })
  return #clients > 0 and clients[1].server_capabilities.documentFormattingProvider == true
end, 500)

if not has_format then
  io.stderr:write("NO_FORMAT_CAPABILITY\n")
  vim.cmd("cquit 1")
  return
end

io.stderr:write("ATTACHED_WITH_FORMATTING\n")
vim.cmd("quit")
