import React, { useEffect, useRef, useState } from 'react'
import { Drawer, Segmented } from 'antd'
import { default as AnsiUp } from 'ansi_up';
import YamlEditor from '@/components/yamlEditor'
import { josn2yaml } from '@/utils/tools'

const GenerateDetail = ({ open, currentRecord, handleClose }) => {

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const ansi_up = new AnsiUp();
  const logRef = useRef<HTMLDivElement | null>(null);
  const [activeKey, setActiveKey] = useState('Exec Result')
  const yamlStr = josn2yaml(currentRecord?.result)

  function handleChange(val) {
    setActiveKey(val)
  }

  useEffect(() => {
    if (logRef && logRef.current) {
      const logHtml = ansi_up.ansi_to_html(currentRecord?.logs);
      logRef.current.innerHTML = logHtml;
      logRef.current.style.whiteSpace = 'pre-wrap';
    }
  }, [ansi_up, currentRecord?.logs, logRef]);

  const isExtra = yamlStr?.data?.startsWith('|') || yamlStr?.data?.startsWith('>');
  const yamlData = isExtra ? yamlStr?.data?.substr(1) : yamlStr?.data

  return (
    <Drawer
      title={`${currentRecord?.type} Detail`}
      width="80%"
      open={open}
      onClose={handleClose}
    >
      <div>
        <div style={{ marginBottom: 20 }}>
          <Segmented options={["Exec Result", "Exec Logs"]} value={activeKey} onChange={handleChange} />
        </div>
        {
          activeKey === 'Exec Result' && <div style={{ height: '100%', overflowY: 'scroll' }}>
            <YamlEditor value={yamlData?.trim()} readOnly={true} themeMode='DARK' />
          </div>
        }
        {
          activeKey === 'Exec Logs' && (
            <div style={{ background: '#000', color: '#fff', padding: 20 }}>
              <div ref={logRef}></div>
            </div>
          )
        }

      </div>
    </Drawer>
  )

}

export default GenerateDetail