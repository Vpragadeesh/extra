const std = @import("std");
const cli = @import("cli/mod.zig");
const config = @import("config/mod.zig");
const magnet = @import("protocol/magnet.zig");
const torrent = @import("torrent/mod.zig");

pub const log = std.log;

pub fn main(init: std.process.Init) !void {
    var fixed_buffer = std.heap.ArenaAllocator.init(std.heap.page_allocator);
    defer fixed_buffer.deinit();

    const allocator = fixed_buffer.allocator();

    if (try runGoBackend(init, allocator)) return;

    var args = try cli.parseArgs(allocator, init.minimal.args);
    defer args.deinit(allocator);

    if (args.torrent_path.len == 0) {
        try cli.printHelp();
        return error.InvalidArguments;
    }

    const cfg = try config.load(allocator, args.config_path);
    defer cfg.deinit(allocator);

    var t = try torrent.TorrentManager.init(allocator, cfg);
    defer t.deinit();

    if (std.mem.startsWith(u8, args.torrent_path, "magnet:")) {
        var spec = try magnet.parse(allocator, args.torrent_path);
        defer spec.deinit(allocator);
        try t.downloadFromMagnet(spec);
    } else {
        try t.downloadFromFile(args.torrent_path);
    }
}

fn runGoBackend(init: std.process.Init, allocator: std.mem.Allocator) !bool {
    var argv = std.ArrayList([]const u8).empty;
    defer argv.deinit(allocator);

    try argv.append(allocator, "./fastdown");

    var it = std.process.Args.Iterator.init(init.minimal.args);
    _ = it.skip();
    while (it.next()) |arg| {
        try argv.append(allocator, arg);
    }

    var child = std.process.spawn(init.io, .{
        .argv = argv.items,
        .stdin = .inherit,
        .stdout = .inherit,
        .stderr = .inherit,
    }) catch |err| switch (err) {
        error.FileNotFound, error.PermissionDenied, error.AccessDenied => return false,
        else => return err,
    };

    const term = try child.wait(init.io);
    switch (term) {
        .exited => |code| if (code != 0) return error.InvalidArguments,
        else => return error.InvalidArguments,
    }
    return true;
}
