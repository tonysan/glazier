// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package eventlog provides an interface to the Windows Event Log.
package eventlog

import (
	"errors"
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"github.com/google/winops/winlog/wevtapi"
)

// Handle maps a handle to an event log resource (EVT_HANDLE). Close() must be called to release the handle.
//
// Note that the order in which handles are closed may matter. Parent handles should not be closed until all
// uses of the handles (queries, etc) are complete.
//
// Ref: https://docs.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtclose
type Handle struct {
	handle windows.Handle
}

// Close releases a Handle.
func (h *Handle) Close() {
	if h != nil {
		wevtapi.EvtClose(h.handle)
	}
}

// An Event is a Handle to an event.
type Event Handle

// Close releases an Event.
func (h *Event) Close() {
	if h != nil {
		wevtapi.EvtClose(h.handle)
	}
}

// A RenderContext is a Handle which tracks a Context as returned by EvtCreateRenderContext.
type RenderContext Handle

// Close releases a RenderContext.
func (h *RenderContext) Close() {
	if h != nil {
		wevtapi.EvtClose(h.handle)
	}
}

// A ResultSet is a Handle returned by a Query or Subscription
type ResultSet Handle

// Close releases a ResultSet.
func (h *ResultSet) Close() {
	if h != nil {
		wevtapi.EvtClose(h.handle)
	}
}

// A Session is a Handle returned by OpenSession
type Session Handle

// Close releases a Session.
func (h *Session) Close() {
	if h != nil {
		wevtapi.EvtClose(h.handle)
	}
}

// EvtRenderContextFlags specify which types of values to render from a given event.
//
// Ref: https://docs.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_render_context_flags
type EvtRenderContextFlags uint32

const (
	// EvtRenderContextValues renders specific properties from the event.
	EvtRenderContextValues EvtRenderContextFlags = iota
	// EvtRenderContextSystem renders the system properties under the System element.
	EvtRenderContextSystem
	// EvtRenderContextUser renders all user-defined properties under the UserData or EventData element.
	EvtRenderContextUser
)

// CreateRenderContext creates a context that specifies the information in the event that you want to render.
//
// The RenderContext is used to obtain only a subset of event data when querying events.
// Without a RenderContext, the entirety of the log data will be returned.
//
// Passing one of EvtRenderContextSystem or EvtRenderContextUser (with valuePaths nil)
// will render all properties under the corresponding element (System or User). Passing
// EvtRenderContextValues along with a list of valuePaths allows the caller to obtain individual
// event elements. valuePaths must be well formed XPath expressions. See the documentation
// for EvtCreateRenderContext and EVT_RENDER_CONTEXT_FLAGS for more detail.
//
// Example, rendering all System values:
//		eventlog.CreateRenderContext(eventlog.EvtRenderContextSystem, nil)
//
// Example, rendering specific values:
//		eventlog.CreateRenderContext(eventlog.EvtRenderContextValues, &[]string{
//				"Event/System/TimeCreated/@SystemTime", "Event/System/Provider/@Name"})
//
// Ref: https://docs.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtcreaterendercontext
func CreateRenderContext(flags EvtRenderContextFlags, valuePaths *[]string) (RenderContext, error) {
	rc := RenderContext{}

	pathsPtr := uintptr(0)
	p := []*uint16{}
	if valuePaths != nil {
		for _, v := range *valuePaths {
			ptr, err := syscall.UTF16PtrFromString(v)
			if err != nil {
				return rc, fmt.Errorf("syscall.UTF16PtrFromString(%s): %w", v, err)
			}
			p = append(p, ptr)
		}
		pathsPtr = uintptr(unsafe.Pointer(&p[0]))
	}

	var err error
	rc.handle, err = wevtapi.EvtCreateRenderContext(uint32(len(p)), uintptr(pathsPtr), uint32(flags))
	return rc, err
}

// An EventSet holds one or more event handles.
//
// Close() must be called to release the event handles when finished.
type EventSet struct {
	Events []Event
	Count  uint32
}

// Close releases all events in the EventSet.
func (e *EventSet) Close() {
	for _, evt := range e.Events {
		evt.Close()
	}
}

// Next gets the next event(s) returned by a query or subscription.
//
// Ref: https://docs.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtnext
func Next(handle ResultSet, count uint32, timeout *time.Duration) (EventSet, error) {
	es := EventSet{}

	defaultTimeout := 2000 * time.Millisecond
	if timeout == nil {
		timeout = &defaultTimeout
	}

	// Get handles to events from the result set.
	evts := make([]windows.Handle, count)
	err := wevtapi.EvtNext(
		handle.handle,                  // Handle to query or subscription result set.
		count,                          // The number of events to attempt to retrieve.
		&evts[0],                       // Pointer to the array of event handles.
		uint32(timeout.Milliseconds()), // Timeout in milliseconds to wait.
		0,                              // Reserved. Must be zero.
		&es.Count)                      // The number of handles in the array that are set by the API.
	if err == windows.ERROR_NO_MORE_ITEMS {
		return es, err
	} else if err != nil {
		return es, fmt.Errorf("wevtapi.EvtNext: %w", err)
	}

	for i := 0; i < int(es.Count); i++ {
		es.Events = append(es.Events, Event{handle: evts[i]})
	}

	return es, nil
}

// Query runs a query to retrieve events from a channel or log file that match the specified query criteria.
//
// Session is only required for remote connections; leave as nil for the local log. Flags can be any of
// wevtapi.EVT_QUERY_FLAGS.
//
// The session handle must remain open until all subsequent processing on the query results have completed. Call
// Close() once complete.
//
// Example:
// 	 conn, err := eventlog.Query(nil, "Windows Powershell", "*", wevtapi.EvtQueryReverseDirection)
// 	 if err != nil {
//     return err
//	 }
//	 defer conn.Close()
//
// Ref: https://docs.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtquery
func Query(session *Session, path string, query string, flags uint32) (ResultSet, error) {
	var rs ResultSet
	var err error

	var s windows.Handle
	if session != nil {
		s = session.handle
	}
	rs.handle, err = wevtapi.EvtQuery(s, windows.StringToUTF16Ptr(path), windows.StringToUTF16Ptr(query), flags)
	if err != nil {
		return rs, fmt.Errorf("EvtQuery: %w", err)
	}
	if rs.handle == windows.InvalidHandle {
		return rs, errors.New("invalid query")
	}
	return rs, nil
}

// EvtVariantData models the union inside of the EVT_VARIANT structure.
//
// Ref: https://docs.microsoft.com/en-us/windows/win32/api/winevt/ns-winevt-evt_variant
type EvtVariantData struct {
	BooleanVal    bool
	SByteVal      int8
	Int16Val      int16
	Int32Val      int32
	Int64Val      int64
	ByteVal       uint8
	UInt16Val     uint16
	UInt32Val     uint32
	UInt64Val     uint64
	SingleVal     float32
	DoubleVal     float64
	FileTimeVal   windows.Filetime
	SysTimeVal    windows.Systemtime
	GuidVal       windows.GUID
	StringVal     string
	AnsiStringVal string
	BinaryVal     byte
	SidVal        windows.SID
	SizeTVal      uint32
	BooleanArr    *[]bool
	SByteArr      *[]int8
	Int16Arr      *[]int16
	Int32Arr      *[]int32
	Int64Arr      *[]int64
	ByteArr       *[]uint16
	UInt16Arr     *[]uint16
	UInt32Arr     *[]uint32
	UInt64Arr     *[]uint64
	SingleArr     *[]float32
	DoubleArr     *[]float64
	FileTimeArr   *[]windows.Filetime
	SysTimeArr    *[]windows.Systemtime
	GuidArr       *[]windows.GUID
	StringArr     *[]string
	AnsiStringArr *[]string
	SidArr        *[]windows.SID
	SizeTArr      *[]uint32
	EvtHandleVal  windows.Handle
	XmlVal        string
	XmlValArr     *[]string
}

// EvtVariantType(EVT_VARIANT_TYPE) defines the possible data types of a EVT_VARIANT data item.
//
// Ref: https://docs.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_variant_type
type EvtVariantType uint32

const (
	EvtVarTypeNull EvtVariantType = iota
	EvtVarTypeString
	EvtVarTypeAnsiString
	EvtVarTypeSByte
	EvtVarTypeByte
	EvtVarTypeInt16
	EvtVarTypeUInt16
	EvtVarTypeInt32
	EvtVarTypeUInt32
	EvtVarTypeInt64
	EvtVarTypeUInt64
	EvtVarTypeSingle
	EvtVarTypeDouble
	EvtVarTypeBoolean
	EvtVarTypeBinary
	EvtVarTypeGuid
	EvtVarTypeSizeT
	EvtVarTypeFileTime
	EvtVarTypeSysTime
	EvtVarTypeSid
	EvtVarTypeHexInt32
	EvtVarTypeHexInt64
	EvtVarTypeEvtHandle
	EvtVarTypeEvtXml
)

// EvtVariant (EVT_VARIANT) contains event data or property values.
//
// Ref: https://docs.microsoft.com/en-us/windows/win32/api/winevt/ns-winevt-evt_variant
type EvtVariant struct {
	Count uint32
	Type  EvtVariantType
	Data  EvtVariantData
}
