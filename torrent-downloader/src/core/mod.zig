const std = @import("std");
const bencode = @import("../protocol/bencode.zig");

pub const ResumeData = struct {
    allocator: std.mem.Allocator,
    pieces_downloaded: []bool,
    bitfield: []u8,
    info_hash: [20]u8,
    total_size: u64,

    pub fn init(allocator: std.mem.Allocator, info_hash: [20]u8, piece_count: usize, total_size: u64) !*ResumeData {
        var resume = try allocator.create(ResumeData);
        resume.* = .{
            .allocator = allocator,
            .pieces_downloaded = try allocator.alloc(bool, piece_count),
            .bitfield = try allocator.alloc(u8, (piece_count + 7) / 8),
            .info_hash = info_hash,
            .total_size = total_size,
        };
        @memset(resume.pieces_downloaded, false);
        @memset(resume.bitfield, 0);
        return resume;
    }

    pub fn deinit(self: *ResumeData) void {
        self.allocator.free(self.pieces_downloaded);
        self.allocator.free(self.bitfield);
    }

    pub fn setPieceComplete(self: *ResumeData, index: usize) void {
        if (index < self.pieces_downloaded.len) {
            self.pieces_downloaded[index] = true;
            self.bitfield[index / 8] |= @as(u8, 1) << @intCast(index % 8);
        }
    }

    pub fn isPieceComplete(self: *ResumeData, index: usize) bool {
        if (index < self.pieces_downloaded.len) {
            return self.pieces_downloaded[index];
        }
        return false;
    }

    pub fn save(self: *ResumeData, path: []const u8) !void {
        var dict = std.StringHashMap(bencode.Value).init(self.allocator);
        defer {
            var iter = dict.iterator();
            while (iter.next()) |entry| {
                self.allocator.free(entry.key_ptr.*);
                entry.value_ptr.*.deinit(self.allocator);
            }
            dict.deinit();
        }

        var pieces_list = std.ArrayList(bencode.Value).init(self.allocator);
        for (self.pieces_downloaded, 0..) |complete, i| {
            if (complete) {
                try pieces_list.append(.{ .integer = @intCast(i) });
            }
        }
        try dict.put("pieces", .{ .list = pieces_list.toOwnedSlice() });

        var hash_str: [40]u8 = undefined;
        for (self.info_hash, 0..) |byte, i| {
            _ = std.fmt.formatIntSlice(hash_str[i * 2..i * 2 + 2], byte, 16, .lower, .{
                .width = 2,
                .fill = '0',
            });
        }
        try dict.put("info hash", .{ .string = try self.allocator.dupe(u8, &hash_str) });
        try dict.put("total size", .{ .integer = @intCast(self.total_size) });

        const file = try std.fs.cwd().createFile(path, .{});
        defer file.close();
        try self.writeBencode(file.writer(), &dict);
    }

    fn writeBencode(self: *ResumeData, writer: anytype, dict: *const std.StringHashMap(bencode.Value)) !void {
        try writer.writeAll("d");
        var iter = dict.iterator();
        while (iter.next()) |entry| {
            try writer.writeAll(entry.key_ptr.*);
            try self.writeValue(writer, entry.value_ptr.*);
        }
        try writer.writeAll("e");
    }

    fn writeValue(self: *ResumeData, writer: anytype, val: bencode.Value) !void {
        switch (val) {
            .integer => |n| try writer.print("i{}e", .{n}),
            .string => |s| try writer.print("{}:s", .{ s.len }),
            .list => |list| {
                try writer.writeAll("l");
                for (list) |*v| try self.writeValue(writer, v.*);
                try writer.writeAll("e");
            },
            .dict => |d| {
                try writer.writeAll("d");
                var iter = d.iterator();
                while (iter.next()) |entry| {
                    try writer.writeAll(entry.key_ptr.*);
                    try self.writeValue(writer, entry.value_ptr.*);
                }
                try writer.writeAll("e");
            },
        }
    }

    pub fn load(allocator: std.mem.Allocator, path: []const u8) !?*ResumeData {
        const file = std.fs.cwd().openFile(path, .{}) catch return null;
        defer file.close();

        const data = try file.readToEndAlloc(allocator, 4096);
        defer allocator.free(data);

        const val = bencode.parse(allocator, data) catch return null;
        defer val.deinit(allocator);

        return null;
    }
};