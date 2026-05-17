const std = @import("std");

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