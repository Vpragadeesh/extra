const std = @import("std");

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
            if (i < filled) writer.writeAll("=") else writer.writeAll(" ");
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
        std.debug.print("\n[✓] Download complete\n", .{});
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
        return std.fmt.allocPrint(std.heap.page_allocator, "{d:.0f}{c}", .{size, units[unit_idx]}) catch "";
    } else {
        return std.fmt.allocPrint(std.heap.page_allocator, "{d:.1}{c}", .{size, units[unit_idx]}) catch "";
    }
}