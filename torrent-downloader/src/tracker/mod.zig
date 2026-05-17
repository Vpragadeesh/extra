const std = @import("std");
const bencode = @import("../protocol/bencode.zig");

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
                const ip = try self.allocator.dupe(u8, std.fmt.bytesToIp(&binary[i..i + 4]));
                const port = std.mem.readInt(u16, binary[i + 4..i + 6], .big);
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