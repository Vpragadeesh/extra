const std = @import("std");

pub const PeerWire = struct {
    allocator: std.mem.Allocator,
    socket: std.net.Stream,
    info_hash: [20]u8,
    peer_id: [20]u8,
    bitfield: std.DynamicBitSet,

    pub fn init(allocator: std.mem.Allocator, socket: std.net.Stream, info_hash: [20]u8, piece_count: usize) !PeerWire {
        var peer = PeerWire{
            .allocator = allocator,
            .socket = socket,
            .info_hash = info_hash,
            .peer_id = undefined,
            .bitfield = try std.DynamicBitSet.initEmpty(allocator, piece_count),
        };
        std.crypto.random.bytes(&peer.peer_id);
        return peer;
    }

    pub fn handshake(self: *PeerWire) !void {
        var handshake: [68]u8 = undefined;
        handshake[0] = 19;
        std.mem.copy(u8, handshake[1..21], "BitTorrent protocol");
        handshake[24..44].* = self.info_hash.*;
        handshake[44..64].* = self.peer_id.*;

        try self.socket.writeAll(&handshake);

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
                const bitfield_len = @intCast(len - 1);
                var bitfield_data = try self.allocator.alloc(u8, bitfield_len);
                _ = try self.socket.read(bitfield_data);
                return .{ .bitfield = bitfield_data };
            },
            7 => {
                var block: [12]u8 = undefined;
                _ = try self.socket.read(&block);
                const piece = std.mem.readInt(u32, block[0..4], .big);
                const offset = std.mem.readInt(u32, block[4..8], .big);
                const block_len = @intCast(len - 13);
                var data = try self.allocator.alloc(u8, block_len);
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