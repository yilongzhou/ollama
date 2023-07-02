import { useEffect, useRef, useState } from 'react'
import path from 'path'
import os from 'os'
import { dialog, getCurrentWindow } from '@electron/remote'
import { HiChevronLeft } from 'react-icons/hi'

const { ipcRenderer } = window.require('electron')

const API_URL = 'http://127.0.0.1:7734'

type Message = {
  sender: 'bot' | 'human'
  content: string
}

const userInfo = os.userInfo()

async function generate(prompt: string, model: string, callback: (res: string) => void) {
  const result = await fetch(`${API_URL}/generate`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      prompt,
      model,
    }),
  })

  if (!result.ok) {
    return
  }

  const reader = result.body.getReader()

  while (true) {
    const { done, value } = await reader.read()

    if (done) {
      break
    }

    const decoder = new TextDecoder()
    let str = decoder.decode(value)

    const re = /}\s*{/g
    str = '[' + str.replace(re, '},{') + ']'
    const messages = JSON.parse(str)

    for (const message of messages) {
      const choice = message.choices[0]

      callback(choice.text)

      if (choice.finish_reason === 'stop') {
        break
      }
    }
  }

  return
}

async function getModels() {
  try {
    const result = await fetch(`${API_URL}/models`, {
      headers: {
        'Content-Type': 'application/json',
      },
    })

    const data = await result.json()
    console.log(data)
    return data
  } catch (error) {
    console.error(error)
    return []
  }
}

export default function () {
  const [prompt, setPrompt] = useState('')
  const [messages, setMessages] = useState<Message[]>([])
  const [currentModel, setCurrentModel] = useState('')
  const [models, setModels] = useState<string[]>([])
  const [generating, setGenerating] = useState(false)

  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    const getModelslist = async () => {
      const models = await getModels()
      setModels(models)
    }

    getModelslist()
   }, [])

  function handleInputChange() {
    const textarea = textareaRef.current;
  
    textarea.style.height = ""
    textarea.style.height = Math.min(textarea.scrollHeight, 200) + "px"
  }

  function resetInputChange() {
    const textarea = textareaRef.current;

    textarea.style.height = ""
  }

  function handleGoBack() {
    setCurrentModel('')

    ipcRenderer.send('go-back')
  }  

  return (
    <div className='flex min-h-screen flex-1 flex-col justify-between bg-white'>

      <header className='sticky px-16 sm:px-20 md:px-40 lg:px-60 xl:px-80 top-0 z-40 flex h-14 w-full flex-row items-center border-b border-black/10 bg-white/75 backdrop-blur-md'>
        {models.length > 0 && 
          <>
            {currentModel ? (
              <>
                <HiChevronLeft className='text-2xl cursor-pointer' onClick={handleGoBack} />      
                <div className='text-sm font-medium ml-2 truncate'>
                  {path.basename(currentModel).replace('.bin', '')}
                </div>
              </>
            ) : (
              <div className='text-sm font-medium ml-2 truncate'>Select a model</div>
            )}
          </>
        }
      </header>

      {models.length > 0 && (
        <section className='mx-auto mb-10 w-full px-4 sm:px-20 md:px-40 lg:px-60 xl:px-80 flex-1 break-words'>
          {currentModel ? (
            <>
              {messages.map((m, i) => (
                <div className='my-4 flex gap-4' key={i}>
                  <div className='flex-none pr-1 text-lg'>
                    {m.sender === 'human' ? (
                      <div className='mt-px flex h-6 w-6 items-center justify-center rounded-md bg-neutral-200 text-sm text-neutral-700'>
                        {userInfo.username[0].toUpperCase()}
                      </div>
                    ) : (
                      <div className='mt-0.5 flex h-6 w-6 items-center justify-center rounded-md bg-blue-600 text-sm text-white'>
                        {path.basename(currentModel)[0].toUpperCase()}
                      </div>
                    )}
                  </div>
                  <div className='flex-1 text-gray-800'>
                    {m.content}
                    {m.sender === 'bot' && generating && i === messages.length - 1 && (
                      <span className='blink relative -top-[3px] left-1 text-[10px]'>█</span>
                    )}
                  </div>
                </div>
              ))}
            </>
          ) : (
            <>
              {models.map((model, i) => (
                <div
                  key={i}
                  onClick={() => {
                    setCurrentModel(model)
                    setMessages([])
                  }}
                  className='cursor-pointer hover:bg-gray-100 p-4 border-b border-black/10'
                >
                  {path.basename(model).replace('.bin', '')}
                </div>
              ))}
            </>
          )}
        </section>
      )}
      {models.length === 0 && (
        <section className='flex flex-1 select-none flex-col items-center justify-center pb-20'>
          <h2 className='text-3xl font-light text-neutral-400'>No model selected</h2>
          <button
            onClick={async () => {
              const res = await dialog.showOpenDialog(getCurrentWindow(), {
                properties: ['openFile', 'multiSelections'],
              })
              if (res.canceled) {
                return
              }

              setModels(res.filePaths)
              setCurrentModel(res.filePaths[0])
            }}
            className='rounded-dm my-8 rounded-md bg-blue-600 px-4 py-2 text-sm text-white hover:brightness-110'
          >
            Open file...
          </button>
        </section>
      )}
      <div className='sticky bottom-0 bg-gradient-to-b from-transparent to-white px-4 sm:px-20 md:px-40 lg:px-60 xl:px-80'>
        {currentModel && (
          <textarea
            ref={textareaRef}
            autoFocus
            rows={1}
            maxLength={512}
            value={prompt}
            placeholder='Send a message...'
            onChange={e => {
              setPrompt(e.target.value)
              handleInputChange()
            }}
            className='mx-auto my-4 block w-full resize-none rounded-xl border border-gray-200 px-5 py-3.5 text-[15px] shadow-lg shadow-black/5 focus:outline-none h-auto'
            onKeyDownCapture={async e => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()

                if (generating) {
                  return
                }

                if (!prompt) {
                  return
                }

                await setMessages(messages => {
                  return [...messages, { sender: 'human', content: prompt }, { sender: 'bot', content: '' }]
                })

                setPrompt('')
                resetInputChange()

                setGenerating(true)
                await generate(prompt, currentModel, res => {
                  setMessages(messages => {
                    const last = messages[messages.length - 1]
                    return [...messages.slice(0, messages.length - 1), { ...last, content: last.content + res }]
                  })
                })
                setGenerating(false)
              }
            }}
          ></textarea>
        )}
      </div>
    </div>
  )
}
