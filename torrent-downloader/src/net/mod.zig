const std = @import("std");

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