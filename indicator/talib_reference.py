import pandas as pd
import talib
import numpy as np
import json
import sys

def generate_reference(indicator_name, prices, **kwargs):
    close = np.array(prices['close'])
    high = np.array(prices['high'])
    low = np.array(prices['low'])
    
    if indicator_name == "SMA":
        res = talib.SMA(close, timeperiod=kwargs.get('period', 14))
    elif indicator_name == "EMA":
        res = talib.EMA(close, timeperiod=kwargs.get('period', 14))
    elif indicator_name == "ATR":
        res = talib.ATR(high, low, close, timeperiod=kwargs.get('period', 14))
    elif indicator_name == "RSI":
        res = talib.RSI(close, timeperiod=kwargs.get('period', 14))
    elif indicator_name == "MACD":
        macd, signal, hist = talib.MACD(close, 
                                        fastperiod=kwargs.get('fast', 12), 
                                        slowperiod=kwargs.get('slow', 26), 
                                        signalperiod=kwargs.get('signal', 9))
        return {"macd": np.nan_to_num(macd).tolist(), "signal": np.nan_to_num(signal).tolist(), "hist": np.nan_to_num(hist).tolist()}
    elif indicator_name == "BOL":
        upper, middle, lower = talib.BBANDS(close, 
                                            timeperiod=kwargs.get('period', 20), 
                                            nbdevup=kwargs.get('std', 2), 
                                            nbdevdn=kwargs.get('std', 2))
        return {"upper": np.nan_to_num(upper).tolist(), "middle": np.nan_to_num(middle).tolist(), "lower": np.nan_to_num(lower).tolist()}
    elif indicator_name == "ROC":
        res = talib.ROC(close, timeperiod=kwargs.get('period', 10))
    elif indicator_name == "MIN":
        res = talib.MIN(close, timeperiod=kwargs.get('period', 14))
    elif indicator_name == "MAX":
        res = talib.MAX(close, timeperiod=kwargs.get('period', 14))
    elif indicator_name == "OBV":
        res = talib.OBV(close, np.array(prices['volume']))
    else:
        return None
    
    return np.nan_to_num(res).tolist()

if __name__ == "__main__":
    input_data = json.load(sys.stdin)
    name = input_data['name']
    prices = input_data['prices']
    params = input_data.get('params', {})
    
    result = generate_reference(name, prices, **params)
    print(json.dumps(result))
