import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
  ReferenceLine
} from 'recharts';
import { Card, Segment, Button, ButtonGroup } from 'semantic-ui-react';
import { API, showError } from '../helpers';

const QPSChart = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState([]);
  const [timeUnit, setTimeUnit] = useState('second'); // 'second' 或 'minute'
  const [lastUnitChange, setLastUnitChange] = useState(Date.now()); // 记录最后一次单位切换的时间
  
  // 获取QPS数据
  const fetchQPSData = async () => {
    setLoading(true);
    try {
      const res = await API.get(`/api/rate-limit/qps?unit=${timeUnit}`);
      const { success, message, data } = res.data;
      
      if (success && data) {
        // 处理数据，为每个数据点添加一个时间戳标识
        const processedData = data.map(item => ({
          ...item,
          unitTimestamp: lastUnitChange // 添加单位切换时间戳标识
        }));
        
        // 过滤掉旧的时间单位数据，只保留当前时间单位的数据
        const filteredData = data.length > 0 
          ? [...data.filter(item => item.unitTimestamp === lastUnitChange), ...processedData]
          : processedData;
        
        setData(filteredData);
      } else {
        showError(message || t('general.error'));
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };
  
  // 定时刷新数据
  useEffect(() => {
    fetchQPSData();
    
    const intervalId = setInterval(() => {
      fetchQPSData();
    }, timeUnit === 'second' ? 1000 : 10000); // 秒级数据每秒刷新，分钟级数据每10秒刷新
    
    return () => clearInterval(intervalId);
  }, [timeUnit, lastUnitChange]);
  
  // 切换时间单位
  const handleUnitChange = (unit) => {
    if (unit !== timeUnit) {
      setTimeUnit(unit);
      setData([]); // 清空数据，避免不同时间单位的数据连接在一起
      setLastUnitChange(Date.now()); // 更新单位切换时间戳
    }
  };
  
  // 计算图表的X轴间隔
  const calculateXAxisInterval = () => {
    if (timeUnit === 'second') {
      return data.length > 30 ? 4 : 2; // 秒级数据，每5秒或3秒显示一个刻度
    } else {
      return data.length > 30 ? 9 : 4; // 分钟级数据，每10分钟或5分钟显示一个刻度
    }
  };
  
  // 计算当前时间点
  const getCurrentTimeIndex = () => {
    const now = new Date();
    const currentTime = `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}${timeUnit === 'second' ? `:${String(now.getSeconds()).padStart(2, '0')}` : ''}`;
    
    return data.findIndex(item => item.time === currentTime);
  };
  
  // 获取最新的QPS值
  const getCurrentQPS = () => {
    if (data.length === 0) return 0;
    const latestData = data[data.length - 1];
    return latestData.value || 0;
  };
  
  // 格式化提示文本
  const formatTooltip = (value) => {
    return `${value} ${t('setting.rate_limit.qps_chart.requests_per_second')}`;
  };

  // 计算Y轴的最大值，确保图表有足够的高度
  const calculateYAxisMax = () => {
    if (data.length === 0) return 5;
    
    const maxValue = Math.max(...data.map(item => item.value || 0));
    return Math.max(maxValue + 2, 5); // 至少为5，或者比最大值大2
  };
  
  // 自定义X轴标签，避免遮挡
  const CustomXAxisTick = (props) => {
    const { x, y, payload } = props;
    
    return (
      <g transform={`translate(${x},${y})`}>
        <text 
          x={0} 
          y={0} 
          dy={16} 
          textAnchor="middle" 
          fill="#999" 
          fontSize={10}
          transform="rotate(-35)"
        >
          {payload.value}
        </text>
      </g>
    );
  };

  // 在QPSChart组件中添加一个函数，根据当前时间单位返回正确的请求数文本
  const getRequestRateText = () => {
    return timeUnit === 'second' ? 
      t('setting.rate_limit.qps_chart.qps_second') : // "每秒请求数"
      t('setting.rate_limit.qps_chart.qps_minute');  // "每分钟请求数"
  };

  // 在Tooltip内容中使用这个函数
  // 修改Tooltip组件的内容
  const CustomTooltip = ({ active, payload, label }) => {
    if (active && payload && payload.length) {
      return (
        <div style={{
          backgroundColor: 'white',
          border: 'none',
          borderRadius: '8px',
          boxShadow: '0 4px 20px rgba(0, 0, 0, 0.15)',
          padding: '10px 14px'
        }}>
          <p style={{ color: '#666', marginBottom: '5px' }}>{`时间: ${label}`}</p>
          <p style={{ color: '#4318FF' }}>
            {`${getRequestRateText()}: ${payload[0].value} 请求/${timeUnit === 'second' ? '秒' : '分钟'}`}
          </p>
        </div>
      );
    }
    return null;
  };
  
  return (
    <Card fluid style={{
      boxShadow: '0 4px 20px rgba(0, 0, 0, 0.05)',
      borderRadius: '10px',
      border: 'none'
    }}>
      <Card.Content>
        <Card.Header style={{ 
          display: 'flex', 
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: '15px',
          fontSize: '16px',
          color: '#333'
        }}>
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <span>{t('setting.rate_limit.qps_chart.title')}</span>
            {/* 将当前QPS信息移到标题旁边 */}
            {/* {data.length > 0 && (
              <div style={{ 
                marginLeft: '15px',
                display: 'flex',
                alignItems: 'center',
                backgroundColor: '#f8f9fa',
                padding: '4px 10px',
                borderRadius: '4px'
              }}>
                <span style={{ color: '#6c63FF', fontSize: '13px' }}>
                  {t('setting.rate_limit.qps_chart.qps')}:
                </span>
                <span style={{ 
                  marginLeft: '5px', 
                  fontSize: '14px', 
                  fontWeight: 'bold',
                  color: '#4318FF'
                }}>
                  {getCurrentQPS()}
                </span>
                <span style={{ 
                  marginLeft: '3px', 
                  fontSize: '12px', 
                  color: '#888'
                }}>
                  / {t('setting.rate_limit.qps_chart.second')}
                </span>
              </div>
            )} */}
          </div>
          <ButtonGroup>
            <Button 
              size='tiny'
              style={{
                backgroundColor: timeUnit === 'second' ? '#2185d0' : '#f8f9fa',
                color: timeUnit === 'second' ? 'white' : '#666',
                borderRadius: '4px 0 0 4px',
                boxShadow: 'none',
                fontWeight: 'normal'
              }}
              onClick={() => handleUnitChange('second')}
            >
              {t('setting.rate_limit.qps_chart.second')}
            </Button>
            <Button 
              size='tiny'
              style={{
                backgroundColor: timeUnit === 'minute' ? '#2185d0' : '#f8f9fa',
                color: timeUnit === 'minute' ? 'white' : '#666',
                borderRadius: '0 4px 4px 0',
                boxShadow: 'none',
                fontWeight: 'normal'
              }}
              onClick={() => handleUnitChange('minute')}
            >
              {t('setting.rate_limit.qps_chart.minute')}
            </Button>
          </ButtonGroup>
        </Card.Header>
        
        <div style={{ 
          position: 'relative',
          height: '320px',
          padding: '0'
        }}>
          {loading && (
            <div style={{
              position: 'absolute',
              top: 0,
              left: 0,
              right: 0,
              bottom: 0,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              backgroundColor: 'rgba(255, 255, 255, 0.7)',
              zIndex: 10,
              borderRadius: '8px'
            }}>
              <div className="ui active loader"></div>
            </div>
          )}
          
          {data.length > 0 ? (
            <ResponsiveContainer width="100%" height="100%">
              <LineChart
                data={data}
                margin={{ top: 20, right: 30, left: 10, bottom: 30 }} // 增加底部边距，为倾斜的X轴标签留出空间
              >
                <defs>
                  <linearGradient id="colorQPS" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#4318FF" stopOpacity={0.3}/>
                    <stop offset="95%" stopColor="#4318FF" stopOpacity={0}/>
                  </linearGradient>
                </defs>
                <CartesianGrid 
                  strokeDasharray="3 3" 
                  vertical={false} 
                  stroke="#f0f0f0" 
                />
                <XAxis 
                  dataKey="time" 
                  interval={calculateXAxisInterval()}
                  tick={<CustomXAxisTick />} // 使用自定义的X轴标签组件
                  axisLine={{ stroke: '#eee' }}
                  tickLine={{ stroke: '#eee' }}
                  padding={{ left: 10, right: 10 }}
                  height={40} // 增加X轴高度，为倾斜的标签留出空间
                />
                <YAxis 
                  tick={{ fontSize: 11, fill: '#999' }}
                  axisLine={{ stroke: '#eee' }}
                  tickLine={{ stroke: '#eee' }}
                  allowDecimals={false}
                  domain={[0, calculateYAxisMax()]}
                  width={30}
                />
                <Tooltip 
                  formatter={formatTooltip}
                  labelFormatter={(label) => `时间: ${label}`}
                  contentStyle={{
                    backgroundColor: 'white',
                    border: 'none',
                    borderRadius: '8px',
                    boxShadow: '0 4px 20px rgba(0, 0, 0, 0.15)',
                    padding: '10px 14px'
                  }}
                  itemStyle={{ color: '#4318FF' }}
                  labelStyle={{ color: '#666', marginBottom: '5px' }}
                  content={<CustomTooltip />} // 使用自定义Tooltip
                />
                <Line 
                  type="monotone" 
                  dataKey="value" 
                  name={t('setting.rate_limit.qps_chart.qps')}
                  stroke="#4318FF" 
                  strokeWidth={2.5}
                  dot={false}
                  activeDot={{ r: 6, stroke: '#4318FF', strokeWidth: 1, fill: 'white' }}
                  fill="url(#colorQPS)"
                  connectNulls={false}
                  isAnimationActive={false} // 关闭动画，避免切换时的过渡效果
                />
                {getCurrentTimeIndex() > 0 && (
                  <ReferenceLine 
                    x={data[getCurrentTimeIndex()]?.time} 
                    stroke="#ff4757" 
                    strokeWidth={1.5}
                    strokeDasharray="5 5"
                    label={{ 
                      value: '当前', 
                      position: 'top', 
                      fill: '#ff4757',
                      fontSize: 11
                    }} 
                  />
                )}
              </LineChart>
            </ResponsiveContainer>
          ) : (
            <div style={{ 
              height: '100%', 
              display: 'flex', 
              alignItems: 'center', 
              justifyContent: 'center',
              color: '#999',
              backgroundColor: '#f9f9f9',
              borderRadius: '8px',
              fontSize: '15px'
            }}>
              {t('setting.rate_limit.no_data')}
            </div>
          )}
        </div>
      </Card.Content>
    </Card>
  );
};

export default QPSChart;