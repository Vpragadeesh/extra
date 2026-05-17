const std = @import("std");
const torrent = @import("../torrent/mod.zig");

pub const Storage = struct {
    allocator: std.mem.Allocator,
    files: []std.fs.File,
    file_info: []torrent.FileInfo,
    total_size: u64,

    pub fn init(allocator: std.mem.Allocator, dir: std.fs.Dir, file_info: []torrent.FileInfo, total_size: u64) !*Storage {
        var storage = try allocator.create(Storage);
        storage.* = .{
            .allocator = allocator,
            .files = &.{},
            .file_info = file_info,
            .total_size = total_size,
        };

        storage.files = try allocator.alloc(std.fs.File, file_info.len);
        for (file_info, 0..) |info, i| {
            const full_path = try std.fs.path.join(allocator, &.{ dir.path, info.path });
            defer allocator.free(full_path);

            try dir.makePath(std.fs.path.dirname(full_path) orelse ".");

            var file = try dir.createFile(full_path, .{
                .truncate = false,
                .allow_ctty = false,
            });

            if (info.length > 0) {
                try file.seekTo(info.length - 1);
                try file.writeAll(&.{0});
            }

            storage.files[i] = file;
        }

        return storage;
    }

    pub fn deinit(self: *Storage) void {
        for (self.files) |*f| f.close();
        self.allocator.free(self.files);
    }

    pub fn writePiece(_: *Storage, _: u32, _: u32, _: []const u8) !void {}

    pub fn readPiece(_: *Storage, _: u32, _: u32, _: u32) ![]u8 {
        return &.{};
    }
};