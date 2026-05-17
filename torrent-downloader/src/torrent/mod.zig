const std = @import("std");
const config = @import("../config/mod.zig");
const bencode = @import("../protocol/bencode.zig");
const magnet = @import("../protocol/magnet.zig");
const tracker = @import("../tracker/mod.zig");
const storage = @import("../storage/mod.zig");
const net = @import("../net/mod.zig");
const ui = @import("../ui/mod.zig");

pub const TorrentManager = struct {
    allocator: std.mem.Allocator,
    config: *config.Config,
    pieces: []Piece,
    piece_hashes: [][20]u8,
    files: []FileInfo,
    total_length: u64,
    downloaded: u64 = 0,
    storage: *storage.Storage,
    limiter: net.Limiter,
    running: bool = false,

    pub fn init(allocator: std.mem.Allocator, cfg: *config.Config) !TorrentManager {
        return TorrentManager{
            .allocator = allocator,
            .config = cfg,
            .pieces = &.{},
            .piece_hashes = &.{},
            .files = &.{},
            .total_length = 0,
            .storage = undefined,
            .limiter = net.Limiter.init(0),
        };
    }

    pub fn deinit(self: *TorrentManager) void {
        _ = self;
    }

    pub fn downloadFromMagnet(self: *TorrentManager, spec: magnet.MagnetSpec) !void {
        std.debug.print("[*] Starting magnet download: {s}\n", .{spec.name orelse "unknown"});
        _ = self;
    }

pub fn downloadFromFile(self: *TorrentManager, path: []const u8) !void {
        _ = self;
        std.debug.print("[*] Loading torrent: {s}\n", .{path});
        std.debug.print("[!] Torrent download not yet implemented in this version\n", .{});
    }

    fn downloadLoop(self: *TorrentManager) !void {
        std.debug.print("[*] Download loop starting...\n", .{});
        _ = self;
    }

    fn parseTorrent(self: *TorrentManager, val: *const bencode.Value) !void {
        const info = bencode.getDict(val, "info") orelse return error.InvalidTorrent;

        const name = bencode.getString(info, "name") orelse "unknown";
        const pieces_data = bencode.getString(info, "pieces") orelse return error.NoPieces;

        const files_val = bencode.getList(info, "files");
        var files = std.ArrayList(FileInfo).init(self.allocator);
        var total_size: u64 = 0;

        if (files_val) |fl| {
            var file_offset: u64 = 0;
            for (fl) |f| {
                const size = bencode.getInt(&f, "length") orelse 0;
                const path_list = bencode.getList(&f, "path");
                var path = try self.allocator.alloc(u8, 0);
                if (path_list) |pl| {
                    var parts = std.ArrayList([]const u8).init(self.allocator);
                    for (pl) |p| {
                        if (p.* == .string) {
                            try parts.append(p.string);
                        }
                    }
                    path = try std.fs.path.join(self.allocator, parts.items);
                }
                try files.append(.{ .path = path, .length = size, .offset = file_offset });
                file_offset += size;
            }
            total_size = file_offset;
        } else {
            const size = bencode.getInt(info, "length") orelse 0;
            try files.append(.{ .path = try self.allocator.dupe(u8, name), .length = size, .offset = 0 });
            total_size = size;
        }

        self.files = files.toOwnedSlice();
        self.total_length = total_size;

        const piece_count = pieces_data.len / 20;
        self.piece_hashes = try self.allocator.alloc([20]u8, piece_count);
        @memcpy(std.mem.bytesAsSlice([20]u8, pieces_data), self.piece_hashes);
        self.pieces = try self.allocator.alloc(Piece, piece_count);
        for (self.pieces) |*p| p.* = .{ .state = .pending };
    }
};

pub const Piece = struct {
    state: PieceState,
    data: ?[]u8,
    hash: [20]u8,
};

pub const PieceState = enum {
    pending,
    downloading,
    completed,
    failed,
};

pub const FileInfo = struct {
    path: []const u8,
    length: u64,
    offset: u64,
};