const std = @import("std");

pub const Args = struct {
    torrent_path: []const u8 = "",
    output_dir: []const u8 = "",
    config_path: []const u8 = "configs/config.yaml",
    max_connections: u32 = 100,
    max_speed: i64 = 0,

    pub fn deinit(self: *Args, allocator: std.mem.Allocator) void {
        _ = self;
        _ = allocator;
    }
};

pub fn parseArgs(_: std.mem.Allocator) !Args {
    const args = Args{};
    return args;
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