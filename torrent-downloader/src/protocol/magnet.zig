const std = @import("std");

pub const MagnetSpec = struct {
    info_hash: [20]u8,
    name: ?[]const u8 = null,

    pub fn deinit(self: *MagnetSpec, allocator: std.mem.Allocator) void {
        if (self.name) |n| allocator.free(n);
    }
};

pub fn parse(_: std.mem.Allocator, uri: []const u8) !MagnetSpec {
    if (!std.mem.startsWith(u8, uri, "magnet:?")) {
        return error.InvalidMagnetURI;
    }

    const spec = MagnetSpec{
        .info_hash = [_]u8{0} ** 20,
    };

    std.debug.print("[*] Parsing magnet URI: {s}\n", .{uri});

    return spec;
}

pub fn formatInfoHash(hash: [20]u8) [40]u8 {
    var result: [40]u8 = undefined;
    for (hash, 0..) |byte, i| {
        _ = std.fmt.formatIntSlice(result[i * 2..i * 2 + 2], byte, 16, .lower, .{
            .width = 2,
            .fill = '0',
        });
    }
    return result;
}