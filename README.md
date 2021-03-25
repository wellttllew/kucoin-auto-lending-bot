# kucoin-auto-lending-bot

![](steps.png) 


## Why not use the official auto-lend feature ? 

Quote from [kucoin help](https://support.kucoin.plus/hc/en-us/articles/900002181706-How-to-Lend-Video-): 

> With the "Auto-Lend" enabled, once the system detects that the available balance of the currency in your main account is greater than the reserved amount you entered (detect per hour), it will automatically place the lending order for you according to your settings.  

As the doc says, the official auto-lend checks my balance only once a hour. If someone returns my USDT before the next check, I am missing out some little interets..  

That's why I made this little bot.  

## Run 

> This docker image is automatically built on docker-hub.  

```
docker run -e MIN_DAILY_INT_RATE=0.0012 \
     -e RESERVED_USDT_AMOUNT=1000 \
     -e KUCOIN_API_KEY=xxxx \
     -e KUCOIN_API_SECRET=xxxxx \
     -e KUCOIN_API_PASSPHRASE=xxxx\ 
     wellttllew/kucoin-auto-lending-bot
```

- `MIN_DAILY_INT_RATE`: min daily interest rate  
- `RESERVED_USDT_AMOUNT`: amount of USDT to reserve  
- `KUCOIN_API_KEY`: kucoin api key 
- `KUCOIN_API_SECRET`: kucoin api secret  
- `KUCOIN_API_PASSPHRASE`: kucoin api passphrase   

ps: the api key should have trading permissions. 