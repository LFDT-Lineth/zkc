;;error:3:44-51:malformed target column
(defcolumns (X :u16) (Y :u16) (one :i1 :constant 1))
(deflookup (l1 :unchecked) ((one Y)) ((one (- 1 X))))
