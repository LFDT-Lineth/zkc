(defcolumns (X :i16) (Y :i16) (Z :i16) (C :i16 :constant 1) (one :i1 :constant 1))
(deflookup test ((one X C)) ((one Y Z)))
