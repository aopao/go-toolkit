package container

import (
	"fmt"
	"reflect"
	"sync"
)

// Entity represent a entity in container
type Entity struct {
	lock sync.RWMutex

	key            interface{} // entity key
	initializeFunc interface{} // initializeFunc is a func to initialize entity
	value          interface{}
	typ            reflect.Type
	index          int // the index in the container

	prototype bool
	c         *Container
}

// Value instance value if not initiailzed
func (e *Entity) Value() (interface{}, error) {
	if e.prototype {
		return e.createValue()
	}

	e.lock.Lock()
	defer e.lock.Unlock()

	if e.value == nil {
		val, err := e.createValue()
		if err != nil {
			return nil, err
		}

		e.value = val
	}

	return e.value, nil
}

func (e *Entity) createValue() (interface{}, error) {
	initializeValue := reflect.ValueOf(e.initializeFunc)
	argValues, err := e.c.funcArgs(initializeValue.Type())
	if err != nil {
		return nil, err
	}

	returnValues := reflect.ValueOf(e.initializeFunc).Call(argValues)
	if len(returnValues) <= 0 {
		return nil, ErrInvalidReturnValueCount("expect greater than 0, got 0")
	}

	if len(returnValues) > 1 && !returnValues[1].IsNil() && returnValues[1].Interface() != nil {
		err, ok := returnValues[1].Interface().(error)
		if ok {
			return nil, err
		}
	}

	return returnValues[0].Interface(), nil
}

// Container is a dependency injection container
type Container struct {
	lock sync.RWMutex

	objects      map[interface{}]*Entity
	objectSlices []*Entity
}

// New create a new container
func New() *Container {
	return &Container{
		objects:      make(map[interface{}]*Entity),
		objectSlices: make([]*Entity, 0),
	}
}

// Must if err is not nil, panic it
func (c *Container) Must(err error) {
	if err != nil {
		panic(err)
	}
}

// Prototype bind a prototype
// initialize func(...) (value, error)
func (c *Container) Prototype(initialize interface{}) error {
	return c.Bind(initialize, true)
}

// PrototypeWithKey bind a prototype with key
// initialize func(...) (value, error)
func (c *Container) PrototypeWithKey(key interface{}, initialize interface{}) error {
	return c.BindWithKey(key, initialize, true)
}

// Singleton bind a singleton
// initialize func(...) (value, error)
func (c *Container) Singleton(initialize interface{}) error {
	return c.Bind(initialize, false)
}

// SingletonWithKey bind a singleton with key
// initialize func(...) (value, error)
func (c *Container) SingletonWithKey(key interface{}, initialize interface{}) error {
	return c.BindWithKey(key, initialize, false)
}

// BindValue bing a value to container
func (c *Container) BindValue(key interface{}, value interface{}) error {
	if value == nil {
		return ErrInvalidArgs("value is nil")
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.objects[key]; ok {
		return ErrRepeatedBind("key repeated")
	}

	entity := Entity{
		initializeFunc: nil,
		key:            key,
		typ:            reflect.TypeOf(value),
		value:          value,
		index:          len(c.objectSlices),
		c:              c,
		prototype:      false,
	}

	c.objects[key] = &entity
	c.objectSlices = append(c.objectSlices, &entity)

	return nil
}

// Bind bind a initialize for object
// initialize func(...) (value, error)
func (c *Container) Bind(initialize interface{}, prototype bool) error {
	if !reflect.ValueOf(initialize).IsValid() {
		return ErrInvalidArgs("initialize is nil")
	}

	initializeType := reflect.ValueOf(initialize).Type()
	if initializeType.NumOut() <= 0 {
		return ErrInvalidArgs("expect func return values count greater than 0, but got 0")
	}

	typ := initializeType.Out(0)
	return c.bindWith(typ, typ, initialize, prototype)
}

// BindWithKey bind a initialize for object with a key
// initialize func(...) (value, error)
func (c *Container) BindWithKey(key interface{}, initialize interface{}, prototype bool) error {
	if !reflect.ValueOf(initialize).IsValid() {
		return ErrInvalidArgs("initialize is nil")
	}

	initializeType := reflect.ValueOf(initialize).Type()
	if initializeType.NumOut() <= 0 {
		return ErrInvalidArgs("expect func return values count greater than 0, but got 0")
	}

	return c.bindWith(key, initializeType.Out(0), initialize, prototype)
}

// Resolve inject args for func by callback
// callback func(...)
func (c *Container) Resolve(callback interface{}) error {
	_, err := c.Call(callback)
	return err
}

// Call call a callback function and return it's results
func (c *Container) Call(callback interface{}) ([]interface{}, error) {
	callbackValue := reflect.ValueOf(callback)
	if !callbackValue.IsValid() {
		return nil, ErrInvalidArgs("callback is nil")
	}

	args, err := c.funcArgs(callbackValue.Type())
	if err != nil {
		return nil, err
	}

	returnValues := callbackValue.Call(args)
	results := make([]interface{}, len(returnValues))
	for index, val := range returnValues {
		results[index] = val.Interface()
	}

	return results, nil
}

// Get get instance by key from container
func (c *Container) Get(key interface{}) (interface{}, error) {
	keyReflectType, ok := key.(reflect.Type)
	if !ok {
		keyReflectType = reflect.TypeOf(key)
	}

	for _, obj := range c.objectSlices {

		if obj.key == key || obj.key == keyReflectType {
			return obj.Value()
		}

		if obj.typ.AssignableTo(keyReflectType) {
			return obj.Value()
		}
	}

	return nil, ErrObjectNotFound(fmt.Sprintf("key=%s", key))
}

// MustGet get instance by key from container
func (c *Container) MustGet(key interface{}) interface{} {
	res, err := c.Get(key)
	if err != nil {
		panic(err)
	}

	return res
}

func (c *Container) bindWith(key interface{}, typ reflect.Type, initialize interface{}, prototype bool) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.objects[key]; ok {
		return ErrRepeatedBind("key repeated")
	}

	entity := Entity{
		initializeFunc: initialize,
		key:            key,
		typ:            typ,
		value:          nil,
		index:          len(c.objectSlices),
		c:              c,
		prototype:      prototype,
	}

	c.objects[key] = &entity
	c.objectSlices = append(c.objectSlices, &entity)

	return nil
}

func (c *Container) funcArgs(t reflect.Type) ([]reflect.Value, error) {
	argsSize := t.NumIn()
	argValues := make([]reflect.Value, argsSize)
	for i := 0; i < argsSize; i++ {
		argType := t.In(i)
		val, err := c.instanceOfType(argType)
		if err != nil {
			return argValues, err
		}

		argValues[i] = val
	}

	return argValues, nil
}

func (c *Container) instanceOfType(t reflect.Type) (reflect.Value, error) {
	if reflect.TypeOf(c).AssignableTo(t) {
		return reflect.ValueOf(c), nil
	}

	arg, err := c.Get(t)
	if err != nil {
		return reflect.Value{}, ErrArgNotInstanced(err.Error())
	}

	return reflect.ValueOf(arg), nil
}

func isErrorType(t reflect.Type) bool {
	return t.Implements(reflect.TypeOf((*error)(nil)).Elem())
}

// ErrObjectNotFound is an error object represent object not found
func ErrObjectNotFound(msg string) error {
	return fmt.Errorf("the object can not be found in container: %s", msg)
}

// ErrArgNotInstanced is an erorr object represent arg not instanced
func ErrArgNotInstanced(msg string) error {
	return fmt.Errorf("the arg can not be found in container: %s", msg)
}

// ErrInvalidReturnValueCount is an error object represent return values count not match
func ErrInvalidReturnValueCount(msg string) error {
	return fmt.Errorf("invalid return value count: %s", msg)
}

// ErrRepeatedBind is an error object represent bind a value repeated
func ErrRepeatedBind(msg string) error {
	return fmt.Errorf("can not bind a value with repeated key: %s", msg)
}

// ErrInvalidArgs is an error object represent invalid args
func ErrInvalidArgs(msg string) error {
	return fmt.Errorf("invalid args: %s", msg)
}
