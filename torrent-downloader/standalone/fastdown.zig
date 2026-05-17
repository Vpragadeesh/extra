const std = @import("std");

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

    var manager = try torrent.TorrentManager.init(allocator, cfg);
    defer manager.deinit();

    if (std.mem.startsWith(u8, args.torrent_path, "magnet:")) {
        var spec = try magnet.parse(allocator, args.torrent_path);
        defer spec.deinit(allocator);
        try manager.downloadFromMagnet(spec);
    } else {
        try manager.downloadFromFile(args.torrent_path);
    }
}

const cli = struct {
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
};

const config = struct {
    pub const Config = struct {
        output_dir: []const u8 = "/home/pragadeesh/Videos/",
        max_speed: i64 = 0,
        connections: u32 = 100,
        chunk_size: i64 = 524288,
        timeout: u32 = 30,
        save_resume: bool = true,
        resume_file: []const u8 = ".resume",

        pub fn deinit(self: *Config, allocator: std.mem.Allocator) void {
            _ = self;
            _ = allocator;
        }
    };

    pub fn load(allocator: std.mem.Allocator, path: []const u8) !*Config {
        _ = path;
        const cfg = try allocator.create(Config);
        cfg.* = Config{};
        return cfg;
    }
};

const bencode = struct {
    pub const Value = union(enum) {
        integer: i64,
        string: []const u8,
        list: []Value,
        dict: std.StringHashMap(Value),

        pub fn deinit(self: *Value, allocator: std.mem.Allocator) void {
            switch (self.*) {
                .string => |s| allocator.free(s),
                .list => |list| {
                    for (list) |*v| v.deinit(allocator);
                    allocator.free(list);
                },
                .dict => |dict| {
                    var iter = dict.iterator();
                    while (iter.next()) |entry| {
                        allocator.free(entry.key_ptr.*);
                        entry.value_ptr.*.deinit(allocator);
                    }
                    dict.deinit();
                },
                else => {},
            }
        }
    };

    pub const ParseError = error{
        InvalidFormat,
        InvalidInteger,
        InvalidString,
        InvalidDictionary,
        UnexpectedEnd,
    };

    pub fn parse(allocator: std.mem.Allocator, data: []const u8) ParseError!Value {
        var index: usize = 0;
        return try parseValue(allocator, data, &index);
    }

    fn parseValue(allocator: std.mem.Allocator, data: []const u8, index: *usize) ParseError!Value {
        if (index.* >= data.len) return error.UnexpectedEnd;

        switch (data[index.*]) {
            'i' => return parseInteger(allocator, data, index),
            'l' => return parseList(allocator, data, index),
            'd' => return parseDict(allocator, data, index),
            '0'...'9' => return parseString(allocator, data, index),
            else => return error.InvalidFormat,
        }
    }

    fn parseInteger(_: std.mem.Allocator, data: []const u8, index: *usize) ParseError!Value {
        index.* += 1;
        const start = index.*;
        while (index.* < data.len and data[index.*] != 'e') {
            index.* += 1;
        }
        if (index.* >= data.len) return error.UnexpectedEnd;
        index.* += 1;

        const num_str = data[start .. index.* - 1];
        const num = std.fmt.parseInt(i64, num_str, 10) catch return error.InvalidInteger;
        return Value{ .integer = num };
    }

    fn parseString(allocator: std.mem.Allocator, data: []const u8, index: *usize) ParseError!Value {
        const colon_idx = std.mem.indexOf(u8, data[index.*..], ":") orelse return error.InvalidString;
        const len_str = data[index.* .. index.* + colon_idx];
        const len = std.fmt.parseInt(usize, len_str, 10) catch return error.InvalidString;

        index.* += colon_idx + 1;
        if (index.* + len > data.len) return error.UnexpectedEnd;

        const str = try allocator.dupe(u8, data[index.* .. index.* + len]);
        index.* += len;
        return Value{ .string = str };
    }

    fn parseList(allocator: std.mem.Allocator, data: []const u8, index: *usize) ParseError!Value {
        index.* += 1;
        var list = std.ArrayList(Value).init(allocator);

        while (index.* < data.len and data[index.*] != 'e') {
            const val = try parseValue(allocator, data, index);
            try list.append(val);
        }

        if (index.* >= data.len) return error.UnexpectedEnd;
        index.* += 1;

        return Value{ .list = list.toOwnedSlice() };
    }

    fn parseDict(allocator: std.mem.Allocator, data: []const u8, index: *usize) ParseError!Value {
        index.* += 1;
        var dict = std.StringHashMap(Value).init(allocator);

        while (index.* < data.len and data[index.*] != 'e') {
            const key_val = try parseString(allocator, data, index);
            const key = key_val.string;
            const value = try parseValue(allocator, data, index);
            try dict.put(key, value);
        }

        if (index.* >= data.len) return error.UnexpectedEnd;
        index.* += 1;

        return Value{ .dict = dict };
    }

    pub fn getString(val: *const Value, key: []const u8) ?[]const u8 {
        if (val.* != .dict) return null;
        const dict = val.dict;
        const entry = dict.get(key) orelse return null;
        if (entry.* != .string) return null;
        return entry.string;
    }

    pub fn getInt(val: *const Value, key: []const u8) ?i64 {
        if (val.* != .dict) return null;
        const dict = val.dict;
        const entry = dict.get(key) orelse return null;
        if (entry.* != .integer) return null;
        return entry.integer;
    }

    pub fn getList(val: *const Value, key: []const u8) ?[]Value {
        if (val.* != .dict) return null;
        const dict = val.dict;
        const entry = dict.get(key) orelse return null;
        if (entry.* != .list) return null;
        return entry.list;
    }

    pub fn getDict(val: *const Value, key: []const u8) ?*const std.StringHashMap(Value) {
        if (val.* != .dict) return null;
        const dict = val.dict;
        const entry = dict.get(key) orelse return null;
        if (entry.* != .dict) return null;
        return &entry.dict;
    }
};

const magnet = struct {
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
            _ = std.fmt.formatIntSlice(result[i * 2 .. i * 2 + 2], byte, 16, .lower, .{
                .width = 2,
                .fill = '0',
            });
        }
        return result;
    }
};

const net = struct {
    pub const Limiter = struct {
        max_bytes_per_sec: i64,
        bytes_sent: i64 = 0,

        pub fn init(max_bytes_per_sec: i64) Limiter {
            return .{
                .max_bytes_per_sec = max_bytes_per_sec,
            };
        }

        pub fn wait(self: *Limiter, bytes: i64) void {
            if (self.max_bytes_per_sec <= 0) return;
            self.bytes_sent += bytes;
        }
    };

    pub const RateLimiter = struct {
        max_bytes_per_sec: i64,
        bytes_used: i64 = 0,

        pub fn init(max_bytes_per_sec: i64) RateLimiter {
            return .{
                .max_bytes_per_sec = max_bytes_per_sec,
            };
        }

        pub fn acquire(self: *RateLimiter, bytes: i64) void {
            if (self.max_bytes_per_sec <= 0) return;
            self.bytes_used += bytes;
        }
    };

    pub const ConnectionPool = struct {
        allocator: std.mem.Allocator,
        max_connections: usize,
        active: usize = 0,

        pub fn init(allocator: std.mem.Allocator, max_connections: usize) !*ConnectionPool {
            const pool = try allocator.create(ConnectionPool);
            pool.* = .{
                .allocator = allocator,
                .max_connections = max_connections,
            };
            return pool;
        }

        pub fn deinit(self: *ConnectionPool) void {
            _ = self;
        }

        pub fn acquire(self: *ConnectionPool) !void {
            while (self.active >= self.max_connections) {
                std.time.sleep(100_000_000);
            }
            self.active += 1;
        }

        pub fn release(self: *ConnectionPool) void {
            if (self.active > 0) self.active -= 1;
        }
    };
};

const tracker = struct {
    pub const TrackerClient = struct {
        allocator: std.mem.Allocator,
        tracker_url: []const u8,
        info_hash: [20]u8,
        peer_id: [20]u8,
        port: u16,
        uploaded: i64 = 0,
        downloaded: i64 = 0,
        left: i64 = 0,

        pub fn init(allocator: std.mem.Allocator, url: []const u8, info_hash: [20]u8) !TrackerClient {
            var client = TrackerClient{
                .allocator = allocator,
                .tracker_url = try allocator.dupe(u8, url),
                .info_hash = info_hash,
                .peer_id = [_]u8{0} ** 20,
                .port = 6881,
            };
            std.crypto.random.bytes(&client.peer_id);
            return client;
        }

        pub fn deinit(self: *TrackerClient) void {
            self.allocator.free(self.tracker_url);
        }

        pub fn announce(self: *TrackerClient) !TrackerResponse {
            const query = try self.buildQuery();
            defer self.allocator.free(query);

            const uri = try std.fmt.allocPrint(self.allocator, "{}?{}", .{
                self.tracker_url, query,
            });
            defer self.allocator.free(uri);

            const uri_parsed = std.Uri.parse(uri) catch return error.InvalidUrl;
            var client = std.http.Client{ .allocator = self.allocator };
            defer client.deinit();

            var req = try client.open(.GET, uri_parsed, .{});
            defer req.deinit();

            try req.send();
            try req.finish();
            try req.readBody();

            const body = try req.reader().readAllAlloc(self.allocator, 4096 * 1024);
            defer self.allocator.free(body);

            return try self.parseResponse(body);
        }

        fn buildQuery(self: *TrackerClient) ![]const u8 {
            var buf = std.ArrayList(u8).init(self.allocator);
            try std.fmt.format(&buf, "info_hash={}", std.fmt.fmtSliceEscape(&self.info_hash));
            try std.fmt.format(&buf, "&peer_id={}", std.fmt.fmtSliceEscape(&self.peer_id));
            try std.fmt.format(&buf, "&port={}", self.port);
            try std.fmt.format(&buf, "&uploaded={}", self.uploaded);
            try std.fmt.format(&buf, "&downloaded={}", self.downloaded);
            try std.fmt.format(&buf, "&left={}", self.left);
            try std.fmt.format(&buf, "&compact=1");
            return buf.toOwnedSlice();
        }

        fn parseResponse(self: *TrackerClient, data: []const u8) !TrackerResponse {
            const val = try bencode.parse(self.allocator, data);
            defer val.deinit(self.allocator);

            const failure = bencode.getString(&val, "failure reason");
            if (failure != null) {
                return error.TrackerFailure;
            }

            const interval = bencode.getInt(&val, "interval") orelse 1800;
            var peers = std.ArrayList(PeerInfo).init(self.allocator);

            const peers_val = bencode.getList(&val, "peers");
            if (peers_val) |pl| {
                for (pl) |p| {
                    if (p.* == .dict) {
                        const ip = bencode.getString(&p, "ip") orelse continue;
                        const port = bencode.getInt(&p, "port") orelse 0;
                        const peer_id = bencode.getString(&p, "peer id");
                        try peers.append(.{
                            .ip = try self.allocator.dupe(u8, ip),
                            .port = @intCast(port),
                            .peer_id = if (peer_id) |id| id[0..20].* else undefined,
                        });
                    }
                }
            } else if (bencode.getString(&val, "peeds")) |binary| {
                var i: usize = 0;
                while (i + 6 <= binary.len) : (i += 6) {
                    const ip = try self.allocator.dupe(u8, std.fmt.bytesToIp(&binary[i .. i + 4]));
                    const port = std.mem.readInt(u16, binary[i + 4 .. i + 6], .big);
                    try peers.append(.{ .ip = ip, .port = port });
                }
            }

            return TrackerResponse{
                .interval = @intCast(interval),
                .peers = peers.toOwnedSlice(),
            };
        }
    };

    pub const TrackerResponse = struct {
        interval: u32,
        peers: []PeerInfo,

        pub fn deinit(self: *TrackerResponse, allocator: std.mem.Allocator) void {
            for (self.peers) |p| allocator.free(p.ip);
            allocator.free(self.peers);
        }
    };

    pub const PeerInfo = struct {
        ip: []const u8,
        port: u16,
        peer_id: [20]u8 = undefined,
    };
};

const peer = struct {
    pub const PeerWire = struct {
        allocator: std.mem.Allocator,
        socket: std.net.Stream,
        info_hash: [20]u8,
        peer_id: [20]u8,
        bitfield: std.DynamicBitSet,

        pub fn init(allocator: std.mem.Allocator, socket: std.net.Stream, info_hash: [20]u8, piece_count: usize) !PeerWire {
            var peer_wire = PeerWire{
                .allocator = allocator,
                .socket = socket,
                .info_hash = info_hash,
                .peer_id = undefined,
                .bitfield = try std.DynamicBitSet.initEmpty(allocator, piece_count),
            };
            std.crypto.random.bytes(&peer_wire.peer_id);
            return peer_wire;
        }

        pub fn handshake(self: *PeerWire) !void {
            var handshake_bytes: [68]u8 = undefined;
            handshake_bytes[0] = 19;
            std.mem.copy(u8, handshake_bytes[1..21], "BitTorrent protocol");
            handshake_bytes[24..44].* = self.info_hash.*;
            handshake_bytes[44..64].* = self.peer_id.*;

            try self.socket.writeAll(&handshake_bytes);

            var response: [68]u8 = undefined;
            _ = try self.socket.read(&response);

            if (response[0] != 19) return error.InvalidProtocol;
            if (!std.mem.eql(u8, response[1..21], "BitTorrent protocol")) return error.InvalidProtocol;
            if (!std.mem.eql(u8, response[24..44], &self.info_hash)) return error.WrongInfoHash;
        }

        pub fn sendInterested(self: *PeerWire) !void {
            const msg: [5]u8 = .{ 0, 0, 0, 1, 2 };
            try self.socket.writeAll(&msg);
        }

        pub fn sendUnchoke(self: *PeerWire) !void {
            const msg: [5]u8 = .{ 0, 0, 0, 1, 1 };
            try self.socket.writeAll(&msg);
        }

        pub fn sendRequest(self: *PeerWire, index: u32, begin: u32, length: u32) !void {
            var msg: [17]u8 = undefined;
            std.mem.writeInt(u32, msg[0..4], 13, .big);
            msg[4] = 6;
            std.mem.writeInt(u32, msg[5..9], index, .big);
            std.mem.writeInt(u32, msg[9..13], begin, .big);
            std.mem.writeInt(u32, msg[13..17], length, .big);
            try self.socket.writeAll(&msg);
        }

        pub fn readMessage(self: *PeerWire) !?PeerMessage {
            var header: [4]u8 = undefined;
            _ = try self.socket.read(&header);
            const len = std.mem.readInt(u32, header[0..4], .big);

            if (len == 0) return null;

            var payload: [1]u8 = undefined;
            _ = try self.socket.read(&payload);
            const id = payload[0];

            switch (id) {
                0 => return .choke,
                1 => return .unchoke,
                4 => return .have,
                5 => {
                    const bitfield_len: usize = @intCast(len - 1);
                    const bitfield_data = try self.allocator.alloc(u8, bitfield_len);
                    _ = try self.socket.read(bitfield_data);
                    return .{ .bitfield = bitfield_data };
                },
                7 => {
                    var block: [12]u8 = undefined;
                    _ = try self.socket.read(&block);
                    const piece = std.mem.readInt(u32, block[0..4], .big);
                    const offset = std.mem.readInt(u32, block[4..8], .big);
                    const block_len: usize = @intCast(len - 13);
                    const data = try self.allocator.alloc(u8, block_len);
                    _ = try self.socket.read(data);
                    return .{ .piece = .{
                        .index = piece,
                        .begin = offset,
                        .data = data,
                    } };
                },
                else => return null,
            }
        }
    };

    pub const PeerMessage = union(enum) {
        choke,
        unchoke,
        have: u32,
        bitfield: []const u8,
        piece: PieceData,
    };

    pub const PieceData = struct {
        index: u32,
        begin: u32,
        data: []u8,
    };

    pub const MessageType = enum(u8) {
        choke = 0,
        unchoke = 1,
        interested = 2,
        not_interested = 3,
        have = 4,
        bitfield = 5,
        request = 6,
        piece = 7,
        cancel = 8,
    };
};

const FileInfo = struct {
    path: []const u8,
    length: u64,
    offset: u64,
};

const storage = struct {
    pub const Storage = struct {
        allocator: std.mem.Allocator,
        files: []std.fs.File,
        file_info: []FileInfo,
        total_size: u64,

        pub fn init(allocator: std.mem.Allocator, dir: std.fs.Dir, file_info: []FileInfo, total_size: u64) !*Storage {
            var store = try allocator.create(Storage);
            store.* = .{
                .allocator = allocator,
                .files = &.{},
                .file_info = file_info,
                .total_size = total_size,
            };

            store.files = try allocator.alloc(std.fs.File, file_info.len);
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

                store.files[i] = file;
            }

            return store;
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
};

const core = struct {
    pub const ResumeData = struct {
        allocator: std.mem.Allocator,
        pieces_downloaded: []bool,
        bitfield: []u8,
        info_hash: [20]u8,
        total_size: u64,

        pub fn init(allocator: std.mem.Allocator, info_hash: [20]u8, piece_count: usize, total_size: u64) !*ResumeData {
            const resume_data = try allocator.create(ResumeData);
            resume_data.* = .{
                .allocator = allocator,
                .pieces_downloaded = try allocator.alloc(bool, piece_count),
                .bitfield = try allocator.alloc(u8, (piece_count + 7) / 8),
                .info_hash = info_hash,
                .total_size = total_size,
            };
            @memset(resume_data.pieces_downloaded, false);
            @memset(resume_data.bitfield, 0);
            return resume_data;
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
                _ = std.fmt.formatIntSlice(hash_str[i * 2 .. i * 2 + 2], byte, 16, .lower, .{
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
                .string => |s| try writer.print("{}:s", .{s.len}),
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
};

const ui = struct {
    pub const Progress = struct {
        allocator: std.mem.Allocator,
        total: u64,
        downloaded: u64 = 0,
        start_time: i128,
        last_update: i128,

        pub fn init(allocator: std.mem.Allocator, total: u64) !*Progress {
            const prog = try allocator.create(Progress);
            prog.* = .{
                .allocator = allocator,
                .total = total,
                .start_time = std.time.nanoTimestamp(),
                .last_update = 0,
            };
            return prog;
        }

        pub fn deinit(self: *Progress) void {
            self.allocator.destroy(self);
        }

        pub fn update(self: *Progress, downloaded: u64) void {
            self.downloaded = downloaded;
            const now = std.time.nanoTimestamp();
            if (now - self.last_update > 200_000_000) {
                self.render();
                self.last_update = now;
            }
        }

        fn render(self: *Progress) void {
            const elapsed: i64 = @intCast((std.time.nanoTimestamp() - self.start_time) / 1_000_000_000);
            const speed = if (elapsed > 0) self.downloaded / @as(u64, @intCast(elapsed)) else 0;
            const pct = if (self.total > 0) @as(f64, @floatFromInt(self.downloaded)) / @as(f64, @floatFromInt(self.total)) * 100 else 0;

            const stdout = std.io.getStdOut();
            const writer = stdout.writer();

            const bar_width = 40;
            const filled: usize = @intCast((bar_width * self.downloaded) / std.math.max(self.total, 1));

            writer.print("\r[", .{}) catch {};
            for (0..bar_width) |i| {
                if (i < filled) writer.writeAll("=") catch {} else writer.writeAll(" ") catch {};
            }
            writer.print("] {:5.1}% | {} | {}downloaded", .{
                pct,
                formatSize(speed) catch "?",
                formatSize(self.downloaded) catch "?",
            }) catch {};
        }

        pub fn finish(self: *Progress) void {
            self.downloaded = self.total;
            self.render();
            std.debug.print("\n[done] Download complete\n", .{});
        }
    };

    fn formatSize(bytes: u64) ![]const u8 {
        const units = "BKMGTPE";
        var size = @as(f64, @floatFromInt(bytes));
        var unit_idx: usize = 0;

        while (size >= 1024 and unit_idx < units.len - 1) {
            size /= 1024;
            unit_idx += 1;
        }

        if (unit_idx == 0) {
            return std.fmt.allocPrint(std.heap.page_allocator, "{d:.0f}{c}", .{ size, units[unit_idx] }) catch "";
        } else {
            return std.fmt.allocPrint(std.heap.page_allocator, "{d:.1}{c}", .{ size, units[unit_idx] }) catch "";
        }
    }
};

const torrent = struct {
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
};
