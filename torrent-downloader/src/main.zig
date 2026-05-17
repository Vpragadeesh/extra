const std = @import("std");
const cli = @import("cli/mod.zig");
const config = @import("config/mod.zig");
const magnet = @import("protocol/magnet.zig");
const torrent = @import("torrent/mod.zig");

pub const log = std.log;

pub fn main() !void {
    var fixed_buffer = std.heap.ArenaAllocator.init(std.heap.page_allocator);
    defer fixed_buffer.deinit();

    const allocator = fixed_buffer.allocator();

    const cfg = try config.load(allocator, "configs/config.yaml");
    defer cfg.deinit(allocator);

    var args = try cli.parseArgs(allocator);
    defer args.deinit(allocator);

    if (args.torrent_path.len == 0) {
        try cli.printHelp();
        return error.InvalidArguments;
    }

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