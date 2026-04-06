-- Neovim headless LSP format test
-- Env vars: LUX_CMD, LUX_INPUT, LUX_OUTPUT
--
-- Usage: LUX_CMD="/path/to/lux lsp" LUX_INPUT=in.go LUX_OUTPUT=out.go \
--        nvim --headless -c "luafile this_file.lua"

local lux_cmd_str = vim.env.LUX_CMD
local input_file = vim.env.LUX_INPUT
local output_file = vim.env.LUX_OUTPUT

if not lux_cmd_str or not input_file or not output_file then
	io.stderr:write("ERROR: set LUX_CMD, LUX_INPUT, LUX_OUTPUT env vars\n")
	vim.cmd("cquit 2")
	return
end

local cmd = {}
for word in lux_cmd_str:gmatch("%S+") do
	table.insert(cmd, word)
end

-- Copy input to output (work on the copy)
local f = io.open(input_file, "r")
if not f then
	io.stderr:write("ERROR: cannot read " .. input_file .. "\n")
	vim.cmd("cquit 1")
	return
end
local content = f:read("*a")
f:close()
f = io.open(output_file, "w")
f:write(content)
f:close()

-- Configure and start LSP
vim.lsp.config("lux", {
	cmd = cmd,
	root_markers = { "go.mod", ".git" },
	filetypes = { "go" },
})
vim.lsp.enable("lux")

-- Open the output file
vim.cmd("edit " .. vim.fn.fnameescape(output_file))

-- Wait for LSP to attach and have formatting capability, then format
local attached = vim.wait(80000, function()
	local clients = vim.lsp.get_clients({ name = "lux", bufnr = 0 })
	return #clients > 0 and clients[1].server_capabilities.documentFormattingProvider
end, 500)

if not attached then
	io.stderr:write("ERROR: LSP not ready after 80s\n")
	vim.cmd("cquit 1")
	return
end

-- Retry formatting until we get edits (backend LSP may still be initializing)
local client = vim.lsp.get_clients({ name = "lux", bufnr = 0 })[1]
local edits = nil
local max_attempts = 10

for attempt = 1, max_attempts do
	vim.wait(3000, function()
		return false
	end, 100)
	local params = vim.lsp.util.make_formatting_params()
	local result = client:request_sync("textDocument/formatting", params, 30000, 0)

	if result and result.err then
		io.stderr:write("FORMAT ERROR (attempt " .. attempt .. "): " .. vim.inspect(result.err) .. "\n")
		vim.cmd("cquit 1")
		return
	end

	if result and result.result and #result.result > 0 then
		edits = result
		break
	end
	io.stderr:write("attempt " .. attempt .. ": no edits, retrying...\n")
end

if edits and edits.result and #edits.result > 0 then
	vim.lsp.util.apply_text_edits(edits.result, vim.api.nvim_get_current_buf(), client.offset_encoding)
else
	io.stderr:write("ERROR: no formatting edits after " .. max_attempts .. " attempts\n")
	vim.cmd("cquit 1")
	return
end

vim.cmd("write")
vim.cmd("quit")
