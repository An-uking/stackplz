package event

import (
    "encoding/binary"
    "fmt"
    "stackplz/user/config"
    "stackplz/user/util"
    "strings"
)

type Timespec struct {
    TvSec  uint64 /* seconds */
    TvNsec uint64 /* nanoseconds */
}

func (this *Timespec) String() string {
    return fmt.Sprintf("seconds=%d,nanoseconds=%d", this.TvSec, this.TvNsec)
}

type SyscallEvent struct {
    ContextEvent
    WaitExit     bool
    UUID         string
    Stackinfo    string
    RegsBuffer   RegsBuf
    UnwindBuffer UnwindBuf
    nr_point     *config.SysCallArgs
    nr           config.Arg_nr
    lr           config.Arg_reg
    sp           config.Arg_reg
    pc           config.Arg_reg
    ret          uint64
    args         [6]uint64
    arg_str      string
}

type Arg_bytes = config.Arg_str

func (this *SyscallEvent) ParseContextSysEnter() (err error) {
    if err = binary.Read(this.buf, binary.LittleEndian, &this.lr); err != nil {
        panic(fmt.Sprintf("binary.Read err:%v", err))
    }
    if err = binary.Read(this.buf, binary.LittleEndian, &this.pc); err != nil {
        panic(fmt.Sprintf("binary.Read err:%v", err))
    }
    if err = binary.Read(this.buf, binary.LittleEndian, &this.sp); err != nil {
        panic(fmt.Sprintf("binary.Read err:%v", err))
    }
    // 根据调用号解析剩余参数
    point := config.GetWatchPointByNR(this.nr.Value)
    nr_point, ok := (point).(*config.SysCallArgs)
    if !ok {
        panic(fmt.Sprintf("cast nr[%d] point to SysCallArgs failed", this.nr.Value))
    }
    this.nr_point = nr_point
    var results []string
    for _, point_arg := range this.nr_point.Args {
        // this.logger.Printf(".... AliasType:%d %d %d", point_arg.AliasType, this.EventId, point_arg.ReadFlag)
        var ptr config.Arg_reg
        if err = binary.Read(this.buf, binary.LittleEndian, &ptr); err != nil {
            panic(fmt.Sprintf("binary.Read err:%v", err))
        }
        base_arg_str := fmt.Sprintf("%s=0x%x", point_arg.ArgName, ptr.Address)
        point_arg.SetValue(base_arg_str)
        if point_arg.Type == config.TYPE_NUM {
            // 目前会全部输出为 hex 后续优化改进
            results = append(results, point_arg.ArgValue)
            continue
        }
        // 这一类参数要等执行结束后读取 这里只获取参数所对应的寄存器值就可以了
        if point_arg.ReadFlag == config.SYS_EXIT {
            results = append(results, point_arg.ArgValue)
            continue
        }
        this.ParseArgByType(&point_arg, ptr)
        results = append(results, point_arg.ArgValue)
    }
    // if !this.WaitExit {
    //     var results []string
    //     for _, point_arg := range this.nr_point.Args {
    //         results = append(results, point_arg.ArgValue)
    //     }
    //     this.arg_str = "(" + strings.Join(results, ", ") + ")"
    // }
    this.arg_str = "(" + strings.Join(results, ", ") + ")"
    return nil
}

func (this *SyscallEvent) ParseContextSysExit() (err error) {
    point := config.GetWatchPointByNR(this.nr.Value)
    nr_point, ok := (point).(*config.SysCallArgs)
    if !ok {
        panic(fmt.Sprintf("cast nr[%d] point to SysCallArgs failed", this.nr.Value))
    }
    this.nr_point = nr_point
    var results []string
    for _, point_arg := range this.nr_point.Args {
        var ptr config.Arg_reg
        if err = binary.Read(this.buf, binary.LittleEndian, &ptr); err != nil {
            this.logger.Printf("SyscallEvent EventId:%d RawSample:\n%s", this.EventId, util.HexDump(this.rec.RawSample, util.COLORRED))
            panic(fmt.Sprintf("binary.Read %d %s err:%v", this.nr.Value, util.B2STrim(this.Comm[:]), err))
        }
        base_arg_str := fmt.Sprintf("%s=0x%x", point_arg.ArgName, ptr.Address)
        point_arg.SetValue(base_arg_str)
        if point_arg.Type == config.TYPE_NUM {
            results = append(results, point_arg.ArgValue)
            continue
        }
        if point_arg.ReadFlag != config.SYS_EXIT {
            results = append(results, point_arg.ArgValue)
            continue
        }
        this.ParseArgByType(&point_arg, ptr)
        results = append(results, point_arg.ArgValue)
    }
    // 处理返回参数
    var ptr config.Arg_reg
    if err = binary.Read(this.buf, binary.LittleEndian, &ptr); err != nil {
        panic(fmt.Sprintf("binary.Read err:%v", err))
    }
    point_arg := this.nr_point.Ret
    base_arg_str := fmt.Sprintf("0x%x", ptr.Address)
    point_arg.SetValue(base_arg_str)
    if point_arg.Type != config.TYPE_NUM {
        this.ParseArgByType(&point_arg, ptr)
    }
    if len(results) == 0 {
        results = append(results, "(void)")
    }
    this.arg_str = fmt.Sprintf("(%s => %s)", point_arg.ArgValue, strings.Join(results, ", "))
    return nil
}

func (this *SyscallEvent) WaitNextEvent() bool {
    return this.WaitExit
}

// func (this *SyscallEvent) MergeEvent(exit_event IEventStruct) {
//     exit_p, ok := (exit_event).(*SyscallEvent)
//     if !ok {
//         panic("cast event.SYSCALL_EXIT to event.SyscallEvent failed")
//     }
//     var results []string
//     for index, point_arg := range this.nr_point.Args {
//         if point_arg.ReadFlag == config.SYS_EXIT {
//             point_arg = exit_p.nr_point.Args[index]
//         }
//         results = append(results, point_arg.ArgValue)
//     }
//     results = append(results, exit_p.nr_point.Ret.ArgValue)
//     this.arg_str = "(" + strings.Join(results, ", ") + ")"
//     this.WaitExit = false
// }

func (this *SyscallEvent) ParseContext() (err error) {
    this.WaitExit = false
    // this.logger.Printf("SyscallEvent EventId:%d RawSample:\n%s", this.EventId, util.HexDump(this.rec.RawSample, util.COLORRED))
    // 处理参数 常规参数的构成 是 索引 + 值
    if err = binary.Read(this.buf, binary.LittleEndian, &this.nr); err != nil {
        panic(fmt.Sprintf("binary.Read err:%v", err))
    }
    if this.EventId == SYSCALL_ENTER {
        // 是否有不执行 sys_exit 的情况 ?
        // 有的调用耗时 也有可能 要不还是把执行结果分开输出吧
        // this.WaitExit = true
        this.ParseContextSysEnter()
    } else if this.EventId == SYSCALL_EXIT {
        this.ParseContextSysExit()
    } else {
        panic(fmt.Sprintf("SyscallEvent.ParseContext() failed, EventId:%d", this.EventId))
    }
    this.ParsePadding()
    err = this.ParseContextStack()
    if err != nil {
        panic(fmt.Sprintf("ParseContextStack err:%v", err))
    }
    return nil
}

func (this *SyscallEvent) GetUUID() string {
    return fmt.Sprintf("%d|%d|%s", this.Pid, this.Tid, util.B2STrim(this.Comm[:]))
}

func (this *SyscallEvent) String() string {
    var base_str string
    base_str = fmt.Sprintf("[%s] nr:%s%s", this.GetUUID(), this.nr_point.PointName, this.arg_str)
    if this.EventId == SYSCALL_ENTER {
        var lr_str string
        var pc_str string
        if this.mconf.GetOff {
            lr_str = fmt.Sprintf("LR:0x%x(%s)", this.lr.Address, this.GetOffset(this.lr.Address))
            pc_str = fmt.Sprintf("PC:0x%x(%s)", this.pc.Address, this.GetOffset(this.pc.Address))
        } else {
            lr_str = fmt.Sprintf("LR:0x%x", this.lr.Address)
            pc_str = fmt.Sprintf("PC:0x%x", this.pc.Address)
        }
        base_str = fmt.Sprintf("%s %s %s SP:0x%x", base_str, lr_str, pc_str, this.sp.Address)
    }
    if this.EventId == SYSCALL_ENTER {
        base_str = this.GetStackTrace(base_str)
    }
    return base_str
}

func (this *SyscallEvent) Clone() IEventStruct {
    event := new(SyscallEvent)
    return event
}
