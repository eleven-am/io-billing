package billing

import "github.com/redis/go-redis/v9"

const luaReserve = `
local existing = redis.call("GET", KEYS[5])
if existing then
  local used = tonumber(redis.call("GET", KEYS[1]) or "0")
  local reserved = tonumber(redis.call("GET", KEYS[2]) or "0")
  local limit = tonumber(redis.call("GET", KEYS[3]) or "0")
  return {2, existing, used, reserved, limit}
end

local amount = tonumber(ARGV[1])
if (not amount) or amount <= 0 then
  return {-1, "invalid_amount", 0, 0, 0}
end

local used = tonumber(redis.call("GET", KEYS[1]) or "0")
local reserved = tonumber(redis.call("GET", KEYS[2]) or "0")
local limit = tonumber(redis.call("GET", KEYS[3]) or "0")
local enforcement = redis.call("GET", KEYS[4]) or "hard_cap"

if enforcement == "hard_cap" and (used + reserved + amount > limit) then
  return {0, "", used, reserved, limit}
end

local reservationId = ARGV[2]
local reservationTTL = tonumber(ARGV[3])
local opTTL = tonumber(ARGV[4])
local opValue = ARGV[5]

redis.call("INCRBY", KEYS[2], amount)

redis.call("HSET", KEYS[6],
  "id", reservationId,
  "tenant_id", ARGV[6],
  "metric", ARGV[7],
  "amount", tostring(amount),
  "period_start", ARGV[8],
  "period_end", ARGV[9],
  "period_key", ARGV[10],
  "operation_id", ARGV[11],
  "created_at", ARGV[12]
)
redis.call("EXPIRE", KEYS[6], reservationTTL)
redis.call("SET", KEYS[5], opValue, "EX", opTTL)

reserved = reserved + amount
return {1, opValue, used, reserved, limit}
`

const luaIncrement = `
local existing = redis.call("GET", KEYS[4])
if existing then
  local used = tonumber(redis.call("GET", KEYS[1]) or "0")
  local limit = tonumber(redis.call("GET", KEYS[2]) or "0")
  return {2, existing, used, limit}
end

local amount = tonumber(ARGV[1])
if (not amount) or amount <= 0 then
  return {-1, "invalid_amount", 0, 0}
end

local used = tonumber(redis.call("GET", KEYS[1]) or "0")
local limit = tonumber(redis.call("GET", KEYS[2]) or "0")
local enforcement = redis.call("GET", KEYS[3]) or "hard_cap"

if enforcement == "hard_cap" and (used + amount > limit) then
  return {0, "", used, limit}
end

local newUsed = tonumber(redis.call("INCRBY", KEYS[1], amount))
redis.call("SET", KEYS[4], ARGV[2], "EX", tonumber(ARGV[3]))
return {1, ARGV[2], newUsed, limit}
`

const luaCommit = `
local existing = redis.call("GET", KEYS[3])
if existing then
  local used = tonumber(redis.call("GET", KEYS[1]) or "0")
  local reserved = tonumber(redis.call("GET", KEYS[2]) or "0")
  return {2, existing, used, reserved, 0}
end

local actual = tonumber(ARGV[1])
if (not actual) or actual < 0 then
  return {-2, "invalid_amount", 0, 0, 0}
end

local reservedAmount = tonumber(redis.call("HGET", KEYS[4], "amount"))
if not reservedAmount then
  return {-1, "reservation_not_found", 0, 0, 0}
end

local newReserved = tonumber(redis.call("DECRBY", KEYS[2], reservedAmount))
if newReserved < 0 then
  redis.call("SET", KEYS[2], 0)
  newReserved = 0
end
local newUsed = tonumber(redis.call("INCRBY", KEYS[1], actual))
redis.call("DEL", KEYS[4])
redis.call("SET", KEYS[3], ARGV[2], "EX", tonumber(ARGV[3]))
return {1, ARGV[2], newUsed, newReserved, reservedAmount}
`

const luaRelease = `
local existing = redis.call("GET", KEYS[2])
if existing then
  local reserved = tonumber(redis.call("GET", KEYS[1]) or "0")
  return {2, existing, reserved, 0}
end

local reservedAmount = tonumber(redis.call("HGET", KEYS[3], "amount"))
if not reservedAmount then
  return {-1, "reservation_not_found", 0, 0}
end

local newReserved = tonumber(redis.call("DECRBY", KEYS[1], reservedAmount))
if newReserved < 0 then
  redis.call("SET", KEYS[1], 0)
  newReserved = 0
end
redis.call("DEL", KEYS[3])
redis.call("SET", KEYS[2], ARGV[1], "EX", tonumber(ARGV[2]))
return {1, ARGV[1], newReserved, reservedAmount}
`

var (
	reserveScript   = redis.NewScript(luaReserve)
	incrementScript = redis.NewScript(luaIncrement)
	commitScript    = redis.NewScript(luaCommit)
	releaseScript   = redis.NewScript(luaRelease)
)
