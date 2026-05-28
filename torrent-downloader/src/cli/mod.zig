const std = @import("std");

pub const Args = struct {
    torrent_path: []const u8 = "",
    output_dir: []const u8 = "",
    config_path: []const u8 = "configs/config.yaml",
    max_connections: u32 = 100,
    max_speed: i64 = 0,

    pub fn deinit(self: *Args, allocator: std.mem.Allocator) void {
        if (self.torrent_path.len != 0) allocator.free(self.torrent_path);
        if (self.output_dir.len != 0) allocator.free(self.output_dir);
        if (!std.mem.eql(u8, self.config_path, "configs/config.yaml")) allocator.free(self.config_path);
    }
};

pub fn parseArgs(allocator: std.mem.Allocator, process_args: std.process.Args) !Args {
    var it = try std.process.Args.Iterator.initAllocator(process_args, allocator);
    defer it.deinit();

    // skip argv[0]
    _ = it.next();

    var out = Args{};
    var seen_positional = false;

    while (it.next()) |arg_z| {
        const arg = std.mem.sliceTo(arg_z, 0);
        if (std.mem.eql(u8, arg, "-h") or std.mem.eql(u8, arg, "--help")) {
            try printHelp();
            return error.InvalidArguments;
        }

        if (std.mem.eql(u8, arg, "-o") or std.mem.eql(u8, arg, "--output")) {
            const val_z = it.next() orelse return error.InvalidArguments;
            const val = std.mem.sliceTo(val_z, 0);
            out.output_dir = try allocator.dupe(u8, val);
            continue;
        }

        if (std.mem.eql(u8, arg, "-c") or std.mem.eql(u8, arg, "--config")) {
            const val_z = it.next() orelse return error.InvalidArguments;
            const val = std.mem.sliceTo(val_z, 0);
            if (!std.mem.eql(u8, out.config_path, "configs/config.yaml")) allocator.free(out.config_path);
            out.config_path = try allocator.dupe(u8, val);
            continue;
        }

        if (std.mem.eql(u8, arg, "-C") or std.mem.eql(u8, arg, "--connections")) {
            const val_z = it.next() orelse return error.InvalidArguments;
            const val = std.mem.sliceTo(val_z, 0);
            out.max_connections = std.fmt.parseInt(u32, val, 10) catch return error.InvalidArguments;
            continue;
        }

        if (std.mem.startsWith(u8, arg, "-")) {
            // unknown flag
            return error.InvalidArguments;
        }

        if (seen_positional) {
            // only 1 positional supported
            return error.InvalidArguments;
        }
        seen_positional = true;
        out.torrent_path = try allocator.dupe(u8, arg);
    }

    return out;
}

pub fn printHelp() !void {
    std.debug.print(
        \\Fastdown - Fast torrent downloader
        \\
        \\Usage: fastdown [options] <magnet-uri-or-torrent-file>
        \\
        \\Options:
        \\  -o, --output <dir>       Output directory
        \\  -c, --config <path>     Config file path
        \\  -C, --connections <n>   Max concurrent connections
        \\  -h, --help              Show this help
        \\
        \\Examples:
        \\  fastdown "magnet:?xt=urn:btih:..."
        \\  fastdown -o ~/Downloads ./file.torrent
        \\
    , .{});
}
