-- wrk 脚本：每个请求随机挑一个站点域名作为 Host，覆盖全部 N 个站点
local n = tonumber(os.getenv("SITES") or "1000")
math.randomseed(os.time())
request = function()
  local h = "s" .. math.random(0, n - 1) .. ".bench.test"
  return wrk.format("GET", "/", { ["Host"] = h })
end
