const std = @import("std");

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

    const num_str = data[start..index.* - 1];
    const num = std.fmt.parseInt(i64, num_str, 10) catch return error.InvalidInteger;
    return Value{ .integer = num };
}

fn parseString(allocator: std.mem.Allocator, data: []const u8, index: *usize) ParseError!Value {
    const colon_idx = std.mem.indexOf(u8, data[index.*..], ":") orelse return error.InvalidString;
    const len_str = data[index.*..index.* + colon_idx];
    const len = std.fmt.parseInt(usize, len_str, 10) catch return error.InvalidString;

    index.* += colon_idx + 1;
    if (index.* + len > data.len) return error.UnexpectedEnd;

    const str = try allocator.dupe(u8, data[index.*..index.* + len]);
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